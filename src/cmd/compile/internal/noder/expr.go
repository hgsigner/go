// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package noder

import (
	"cmd/compile/internal/base"
	"cmd/compile/internal/ir"
	"cmd/compile/internal/syntax"
	"cmd/compile/internal/typecheck"
	"cmd/compile/internal/types"
	"cmd/compile/internal/types2"
)

func (g *irgen) expr(expr syntax.Expr) ir.Node {
	// TODO(mdempsky): Change callers to not call on nil?
	if expr == nil {
		return nil
	}

	if expr, ok := expr.(*syntax.Name); ok && expr.Value == "_" {
		return ir.BlankNode
	}

	tv, ok := g.info.Types[expr]
	if !ok {
		base.FatalfAt(g.pos(expr), "missing type for %v (%T)", expr, expr)
	}
	switch {
	case tv.IsBuiltin():
		// TODO(mdempsky): Handle in CallExpr?
		return g.use(expr.(*syntax.Name))
	case tv.IsType():
		return ir.TypeNode(g.typ(tv.Type))
	case tv.IsValue(), tv.IsVoid():
		// ok
	default:
		base.FatalfAt(g.pos(expr), "unrecognized type-checker result")
	}

	// The gc backend expects all expressions to have a concrete type, and
	// types2 mostly satisfies this expectation already. But there are a few
	// cases where the Go spec doesn't require converting to concrete type,
	// and so types2 leaves them untyped. So we need to fix those up here.
	typ := tv.Type
	if basic, ok := typ.(*types2.Basic); ok && basic.Info()&types2.IsUntyped != 0 {
		switch basic.Kind() {
		case types2.UntypedNil:
			// ok; can appear in type switch case clauses
			// TODO(mdempsky): Handle as part of type switches instead?
		case types2.UntypedBool:
			typ = types2.Typ[types2.Bool] // expression in "if" or "for" condition
		case types2.UntypedString:
			typ = types2.Typ[types2.String] // argument to "append" or "copy" calls
		default:
			base.FatalfAt(g.pos(expr), "unexpected untyped type: %v", basic)
		}
	}

	// Constant expression.
	if tv.Value != nil {
		return Const(g.pos(expr), g.typ(typ), tv.Value)
	}

	n := g.expr0(typ, expr)
	if n.Typecheck() != 1 {
		base.FatalfAt(g.pos(expr), "missed typecheck: %+v", n)
	}
	if !g.match(n.Type(), typ, tv.HasOk()) {
		base.FatalfAt(g.pos(expr), "expected %L to have type %v", n, typ)
	}
	return n
}

func (g *irgen) expr0(typ types2.Type, expr syntax.Expr) ir.Node {
	pos := g.pos(expr)

	switch expr := expr.(type) {
	case *syntax.Name:
		if _, isNil := g.info.Uses[expr].(*types2.Nil); isNil {
			return Nil(pos, g.typ(typ))
		}
		// TODO(mdempsky): Remove dependency on typecheck.Expr.
		return typecheck.Expr(g.use(expr))

	case *syntax.CompositeLit:
		return g.compLit(typ, expr)
	case *syntax.FuncLit:
		return g.funcLit(typ, expr)

	case *syntax.AssertExpr:
		return Assert(pos, g.expr(expr.X), g.typeExpr(expr.Type))
	case *syntax.CallExpr:
		return Call(pos, g.expr(expr.Fun), g.exprs(expr.ArgList), expr.HasDots)
	case *syntax.IndexExpr:
		return Index(pos, g.expr(expr.X), g.expr(expr.Index))
	case *syntax.ParenExpr:
		return g.expr(expr.X) // skip parens; unneeded after parse+typecheck
	case *syntax.SelectorExpr:
		// Qualified identifier.
		if name, ok := expr.X.(*syntax.Name); ok {
			if _, ok := g.info.Uses[name].(*types2.PkgName); ok {
				// TODO(mdempsky): Remove dependency on typecheck.Expr.
				return typecheck.Expr(g.use(expr.Sel))
			}
		}

		// TODO(mdempsky/danscales): Use g.info.Selections[expr]
		// to resolve field/method selection. See CL 280633.
		return typecheck.Expr(ir.NewSelectorExpr(pos, ir.OXDOT, g.expr(expr.X), g.name(expr.Sel)))
	case *syntax.SliceExpr:
		return Slice(pos, g.expr(expr.X), g.expr(expr.Index[0]), g.expr(expr.Index[1]), g.expr(expr.Index[2]))

	case *syntax.Operation:
		if expr.Y == nil {
			return Unary(pos, g.op(expr.Op, unOps[:]), g.expr(expr.X))
		}
		switch op := g.op(expr.Op, binOps[:]); op {
		case ir.OEQ, ir.ONE, ir.OLT, ir.OLE, ir.OGT, ir.OGE:
			return Compare(pos, g.typ(typ), op, g.expr(expr.X), g.expr(expr.Y))
		default:
			return Binary(pos, op, g.expr(expr.X), g.expr(expr.Y))
		}

	default:
		g.unhandled("expression", expr)
		panic("unreachable")
	}
}

func (g *irgen) exprList(expr syntax.Expr) []ir.Node {
	switch expr := expr.(type) {
	case nil:
		return nil
	case *syntax.ListExpr:
		return g.exprs(expr.ElemList)
	default:
		return []ir.Node{g.expr(expr)}
	}
}

func (g *irgen) exprs(exprs []syntax.Expr) []ir.Node {
	nodes := make([]ir.Node, len(exprs))
	for i, expr := range exprs {
		nodes[i] = g.expr(expr)
	}
	return nodes
}

func (g *irgen) compLit(typ types2.Type, lit *syntax.CompositeLit) ir.Node {
	if ptr, ok := typ.Underlying().(*types2.Pointer); ok {
		n := ir.NewAddrExpr(g.pos(lit), g.compLit(ptr.Elem(), lit))
		n.SetOp(ir.OPTRLIT)
		return typed(g.typ(typ), n)
	}

	_, isStruct := typ.Underlying().(*types2.Struct)

	exprs := make([]ir.Node, len(lit.ElemList))
	for i, elem := range lit.ElemList {
		switch elem := elem.(type) {
		case *syntax.KeyValueExpr:
			if isStruct {
				exprs[i] = ir.NewStructKeyExpr(g.pos(elem), g.name(elem.Key.(*syntax.Name)), g.expr(elem.Value))
			} else {
				exprs[i] = ir.NewKeyExpr(g.pos(elem), g.expr(elem.Key), g.expr(elem.Value))
			}
		default:
			exprs[i] = g.expr(elem)
		}
	}

	// TODO(mdempsky): Remove dependency on typecheck.Expr.
	return typecheck.Expr(ir.NewCompLitExpr(g.pos(lit), ir.OCOMPLIT, ir.TypeNode(g.typ(typ)), exprs))
}

func (g *irgen) funcLit(typ types2.Type, expr *syntax.FuncLit) ir.Node {
	fn := ir.NewFunc(g.pos(expr))
	fn.SetIsHiddenClosure(ir.CurFunc != nil)

	fn.Nname = ir.NewNameAt(g.pos(expr), typecheck.ClosureName(ir.CurFunc))
	ir.MarkFunc(fn.Nname)
	fn.Nname.SetType(g.typ(typ))
	fn.Nname.Func = fn
	fn.Nname.Defn = fn

	fn.OClosure = ir.NewClosureExpr(g.pos(expr), fn)
	fn.OClosure.SetType(fn.Nname.Type())
	fn.OClosure.SetTypecheck(1)

	g.funcBody(fn, nil, expr.Type, expr.Body)

	ir.FinishCaptureNames(fn.Pos(), ir.CurFunc, fn)

	// TODO(mdempsky): ir.CaptureName should probably handle
	// copying these fields from the canonical variable.
	for _, cv := range fn.ClosureVars {
		cv.SetType(cv.Canonical().Type())
		cv.SetTypecheck(1)
		cv.SetWalkdef(1)
	}

	g.target.Decls = append(g.target.Decls, fn)

	return fn.OClosure
}

func (g *irgen) typeExpr(typ syntax.Expr) *types.Type {
	n := g.expr(typ)
	if n.Op() != ir.OTYPE {
		base.FatalfAt(g.pos(typ), "expected type: %L", n)
	}
	return n.Type()
}