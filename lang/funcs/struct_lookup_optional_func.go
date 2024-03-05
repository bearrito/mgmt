// Mgmt
// Copyright (C) 2013-2024+ James Shubin and the project contributors
// Written by James Shubin <james@shubin.ca> and the project contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.
//
// Additional permission under GNU GPL version 3 section 7
//
// If you modify this program, or any covered work, by linking or combining it
// with embedded mcl code and modules (and that the embedded mcl code and
// modules which link with this program, contain a copy of their source code in
// the authoritative form) containing parts covered by the terms of any other
// license, the licensors of this program grant you additional permission to
// convey the resulting work. Furthermore, the licensors of this program grant
// the original author, James Shubin, additional permission to update this
// additional permission if he deems it necessary to achieve the goals of this
// additional permission.

package funcs

import (
	"context"
	"fmt"

	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"
	"github.com/purpleidea/mgmt/util/errwrap"
)

const (
	// StructLookupOptionalFuncName is the name this function is registered
	// as. This starts with an underscore so that it cannot be used from the
	// lexer.
	StructLookupOptionalFuncName = "_struct_lookup_optional"

	// arg names...
	structLookupOptionalArgNameStruct   = "struct"
	structLookupOptionalArgNameField    = "field"
	structLookupOptionalArgNameOptional = "optional"
)

func init() {
	Register(StructLookupOptionalFuncName, func() interfaces.Func { return &StructLookupOptionalFunc{} }) // must register the func and name
}

var _ interfaces.PolyFunc = &StructLookupOptionalFunc{} // ensure it meets this expectation

// StructLookupOptionalFunc is a struct field lookup function. It does a special
// trick in that it will unify on a struct that doesn't have the specified field
// in it, but in that case, it will always return the optional value. This is a
// bit different from the "default" mechanism that is used by list and map
// lookup functions.
type StructLookupOptionalFunc struct {
	Type *types.Type // Kind == Struct, that is used as the struct we lookup
	Out  *types.Type // type of field we're extracting (also the type of optional)

	init  *interfaces.Init
	last  types.Value // last value received to use for diff
	field string

	result types.Value // last calculated output
}

// String returns a simple name for this function. This is needed so this struct
// can satisfy the pgraph.Vertex interface.
func (obj *StructLookupOptionalFunc) String() string {
	return StructLookupOptionalFuncName
}

// ArgGen returns the Nth arg name for this function.
func (obj *StructLookupOptionalFunc) ArgGen(index int) (string, error) {
	seq := []string{structLookupOptionalArgNameStruct, structLookupOptionalArgNameField, structLookupOptionalArgNameOptional}
	if l := len(seq); index >= l {
		return "", fmt.Errorf("index %d exceeds arg length of %d", index, l)
	}
	return seq[index], nil
}

// Unify returns the list of invariants that this func produces.
func (obj *StructLookupOptionalFunc) Unify(expr interfaces.Expr) ([]interfaces.Invariant, error) {
	var invariants []interfaces.Invariant
	var invar interfaces.Invariant

	// func(struct T1, field str, optional T2) T2

	structName, err := obj.ArgGen(0)
	if err != nil {
		return nil, err
	}

	fieldName, err := obj.ArgGen(1)
	if err != nil {
		return nil, err
	}

	optionalName, err := obj.ArgGen(2)
	if err != nil {
		return nil, err
	}

	dummyStruct := &interfaces.ExprAny{}   // corresponds to the struct type
	dummyField := &interfaces.ExprAny{}    // corresponds to the field type
	dummyOptional := &interfaces.ExprAny{} // corresponds to the optional type
	dummyOut := &interfaces.ExprAny{}      // corresponds to the out string

	// field arg type of string
	invar = &interfaces.EqualsInvariant{
		Expr: dummyField,
		Type: types.TypeStr,
	}
	invariants = append(invariants, invar)

	// XXX: we could use this relationship *if* our solver could understand
	// different fields, and partial struct matches. I guess we'll leave it
	// for another day!
	//mapped := make(map[string]interfaces.Expr)
	//ordered := []string{???}
	//mapped[???] = dummyField
	//invar = &interfaces.EqualityWrapStructInvariant{
	//	Expr1:    dummyStruct,
	//	Expr2Map: mapped,
	//	Expr2Ord: ordered,
	//}
	//invariants = append(invariants, invar)

	// These two types should be identical. This is the safest approach. In
	// the case where the struct field is missing, then this should be true,
	// and when it is present, we'll never use the optional value, but we
	// can still enforce it's the same type.
	invar = &interfaces.EqualityInvariant{
		Expr1: dummyOptional,
		Expr2: dummyOut,
	}
	invariants = append(invariants, invar)

	// full function
	mapped := make(map[string]interfaces.Expr)
	ordered := []string{structName, fieldName, optionalName}
	mapped[structName] = dummyStruct
	mapped[fieldName] = dummyField
	mapped[optionalName] = dummyOptional

	invar = &interfaces.EqualityWrapFuncInvariant{
		Expr1:    expr, // maps directly to us!
		Expr2Map: mapped,
		Expr2Ord: ordered,
		Expr2Out: dummyOut,
	}
	invariants = append(invariants, invar)

	// generator function
	fn := func(fnInvariants []interfaces.Invariant, solved map[interfaces.Expr]*types.Type) ([]interfaces.Invariant, error) {
		for _, invariant := range fnInvariants {
			// search for this special type of invariant
			cfavInvar, ok := invariant.(*interfaces.CallFuncArgsValueInvariant)
			if !ok {
				continue
			}
			// did we find the mapping from us to ExprCall ?
			if cfavInvar.Func != expr {
				continue
			}
			// cfavInvar.Expr is the ExprCall! (the return pointer)
			// cfavInvar.Args are the args that ExprCall uses!
			if l := len(cfavInvar.Args); l != 3 {
				return nil, fmt.Errorf("unable to build function with %d args", l)
			}

			var invariants []interfaces.Invariant
			var invar interfaces.Invariant

			// add the relationship to the returned value
			invar = &interfaces.EqualityInvariant{
				Expr1: cfavInvar.Expr,
				Expr2: dummyOut,
			}
			invariants = append(invariants, invar)

			// add the relationships to the called args
			invar = &interfaces.EqualityInvariant{
				Expr1: cfavInvar.Args[0],
				Expr2: dummyStruct,
			}
			invariants = append(invariants, invar)

			invar = &interfaces.EqualityInvariant{
				Expr1: cfavInvar.Args[1],
				Expr2: dummyField,
			}
			invariants = append(invariants, invar)

			invar = &interfaces.EqualityInvariant{
				Expr1: cfavInvar.Args[2],
				Expr2: dummyOptional,
			}
			invariants = append(invariants, invar)

			// second arg must be a string
			invar = &interfaces.EqualsInvariant{
				Expr: cfavInvar.Args[1],
				Type: types.TypeStr,
			}
			invariants = append(invariants, invar)

			// Not necessary for the field to be known or be static!
			var field string
			value, err := cfavInvar.Args[1].Value() // is it known?
			if err == nil {
				if k := value.Type().Kind; k != types.KindStr {
					return nil, fmt.Errorf("unable to build function with 1st arg of kind: %s", k)
				}
				field = value.Str() // must not panic
			}

			// If we figure out both of these types, we'll know the
			var t1 *types.Type // struct type
			var t2 *types.Type // optional / return type

			// validateArg0 checks: struct T1
			validateArg0 := func(typ *types.Type) error {
				if typ == nil { // unknown so far
					return nil
				}

				// we happen to have a struct!
				if k := typ.Kind; k != types.KindStruct {
					return fmt.Errorf("unable to build function with 0th arg of kind: %s", k)
				}

				// check both Ord and Map for safety
				found := false
				for _, s := range typ.Ord {
					if s == field {
						found = true
						break
					}
				}
				t, exists := typ.Map[field] // type found is T2
				if field != "" {
					if !exists || !found {
						//fmt.Printf("might be using optional arg, struct is missing field: %s\n", field)
					} else if err := t.Cmp(t2); t2 != nil && err != nil {
						return errwrap.Wrapf(err, "input type was inconsistent")
					}

					// learn!
					t2 = t
				}

				if err := typ.Cmp(t1); t1 != nil && err != nil {
					return errwrap.Wrapf(err, "input type was inconsistent")
				}

				// learn!
				t1 = typ
				return nil
			}

			validateArg2OrOut := func(typ *types.Type) error {
				if typ == nil { // unknown so far
					return nil
				}

				if err := typ.Cmp(t2); t2 != nil && err != nil {
					return errwrap.Wrapf(err, "input type was inconsistent")
				}

				// learn!
				t2 = typ
				return nil
			}

			if typ, err := cfavInvar.Args[0].Type(); err == nil { // is it known?
				// this sets t1 (and sometimes t2) on success if it learned
				if err := validateArg0(typ); err != nil {
					return nil, errwrap.Wrapf(err, "first struct arg type is inconsistent")
				}
			}
			if typ, exists := solved[cfavInvar.Args[0]]; exists { // alternate way to lookup type
				// this sets t1 (and sometimes t2) on success if it learned
				if err := validateArg0(typ); err != nil {
					return nil, errwrap.Wrapf(err, "first struct arg type is inconsistent")
				}
			}

			if typ, err := cfavInvar.Args[2].Type(); err == nil { // is it known?
				// this sets t2 on success if it learned
				if err := validateArg2OrOut(typ); err != nil {
					return nil, errwrap.Wrapf(err, "third struct arg type is inconsistent")
				}
			}
			if typ, exists := solved[cfavInvar.Args[2]]; exists { // alternate way to lookup type
				// this sets t2 on success if it learned
				if err := validateArg2OrOut(typ); err != nil {
					return nil, errwrap.Wrapf(err, "third struct arg type is inconsistent")
				}
			}

			// look at the return type too (if known)
			if typ, err := cfavInvar.Expr.Type(); err == nil { // is it known?
				// this sets t2 on success if it learned
				if err := validateArg2OrOut(typ); err != nil {
					return nil, errwrap.Wrapf(err, "third struct arg type is inconsistent")
				}
			}
			if typ, exists := solved[cfavInvar.Expr]; exists { // alternate way to lookup type
				// this sets t2 on success if it learned
				if err := validateArg2OrOut(typ); err != nil {
					return nil, errwrap.Wrapf(err, "third struct arg type is inconsistent")
				}
			}

			// XXX: if the struct type/value isn't know statically?

			if t1 != nil {
				invar = &interfaces.EqualsInvariant{
					Expr: dummyStruct,
					Type: t1,
				}
				invariants = append(invariants, invar)

				// We know *some* information about the struct!
				// Let's hope the unusedField expr won't trip
				// up the solver...
				mapped := make(map[string]interfaces.Expr)
				ordered := []string{}
				for _, x := range t1.Ord {
					// We *don't* need to solve unusedField
					unusedField := &interfaces.ExprAny{}
					mapped[x] = unusedField
					if x == field { // the one we care about
						mapped[x] = dummyOut
					}
					ordered = append(ordered, x)
				}
				// We map to dummyOut which is the return type
				// and has the same type of the field we want!
				mapped[field] = dummyOut // redundant =D
				invar = &interfaces.EqualityWrapStructInvariant{
					Expr1:    dummyStruct,
					Expr2Map: mapped,
					Expr2Ord: ordered,
				}
				// We only want to add this weird thing if the
				// field actually exists. Otherwise ignore it.
				if _, exists := t1.Map[field]; field != "" && exists {
					invariants = append(invariants, invar)
				}
			}
			if t2 != nil {
				invar = &interfaces.EqualsInvariant{
					Expr: dummyOptional,
					Type: t2,
				}
				invariants = append(invariants, invar)
				invar = &interfaces.EqualsInvariant{
					Expr: dummyOut,
					Type: t2,
				}
				invariants = append(invariants, invar)
			}

			// XXX: if t1 or t2 are missing, we could also return a
			// new generator for later if we learn new information,
			// but we'd have to be careful to not do it infinitely.

			// TODO: do we return this relationship with ExprCall?
			invar = &interfaces.EqualityWrapCallInvariant{
				// TODO: should Expr1 and Expr2 be reversed???
				Expr1: cfavInvar.Expr,
				//Expr2Func: cfavInvar.Func, // same as below
				Expr2Func: expr,
			}
			invariants = append(invariants, invar)

			// TODO: are there any other invariants we should build?
			return invariants, nil // generator return
		}
		// We couldn't tell the solver anything it didn't already know!
		return nil, fmt.Errorf("couldn't generate new invariants")
	}
	invar = &interfaces.GeneratorInvariant{
		Func: fn,
	}
	invariants = append(invariants, invar)

	return invariants, nil
}

// Build is run to turn the polymorphic, undetermined function, into the
// specific statically typed version. It is usually run after Unify completes,
// and must be run before Info() and any of the other Func interface methods are
// used. This function is idempotent, as long as the arg isn't changed between
// runs.
func (obj *StructLookupOptionalFunc) Build(typ *types.Type) (*types.Type, error) {
	// typ is the KindFunc signature we're trying to build...
	if typ.Kind != types.KindFunc {
		return nil, fmt.Errorf("input type must be of kind func")
	}

	if len(typ.Ord) != 3 {
		return nil, fmt.Errorf("the structlookup function needs exactly three args")
	}
	if typ.Out == nil {
		return nil, fmt.Errorf("return type of function must be specified")
	}
	if typ.Map == nil {
		return nil, fmt.Errorf("invalid input type")
	}

	tStruct, exists := typ.Map[typ.Ord[0]]
	if !exists || tStruct == nil {
		return nil, fmt.Errorf("first arg must be specified")
	}

	tField, exists := typ.Map[typ.Ord[1]]
	if !exists || tField == nil {
		return nil, fmt.Errorf("second arg must be specified")
	}
	if err := tField.Cmp(types.TypeStr); err != nil {
		return nil, errwrap.Wrapf(err, "field must be an str")
	}

	tOptional, exists := typ.Map[typ.Ord[2]]
	if !exists || tOptional == nil {
		return nil, fmt.Errorf("third arg must be specified")
	}
	if err := tOptional.Cmp(typ.Out); err != nil {
		return nil, errwrap.Wrapf(err, "optional arg must match return type")
	}

	// NOTE: We actually don't know which field this is, only its type! we
	// could have cached the discovered field during Polymorphisms(), but it
	// turns out it's not actually necessary for us to know it to build the
	// struct.
	obj.Type = tStruct // struct type
	obj.Out = typ.Out  // type of return value

	return obj.sig(), nil
}

// Validate tells us if the input struct takes a valid form.
func (obj *StructLookupOptionalFunc) Validate() error {
	if obj.Type == nil { // build must be run first
		return fmt.Errorf("type is still unspecified")
	}
	if obj.Type.Kind != types.KindStruct {
		return fmt.Errorf("type must be a kind of struct")
	}
	if obj.Out == nil {
		return fmt.Errorf("return type must be specified")
	}

	// TODO: can we do better and validate more aspects here?

	return nil
}

// Info returns some static info about itself. Build must be called before this
// will return correct data.
func (obj *StructLookupOptionalFunc) Info() *interfaces.Info {
	var sig *types.Type
	if obj.Type != nil { // don't panic if called speculatively
		// TODO: can obj.Out be nil (a partial) ?
		sig = obj.sig() // helper
	}
	return &interfaces.Info{
		Pure: true,
		Memo: false,
		Sig:  sig, // func kind
		Err:  obj.Validate(),
	}
}

// helper
func (obj *StructLookupOptionalFunc) sig() *types.Type {
	return types.NewType(fmt.Sprintf("func(%s %s, %s str, %s %s) %s", structLookupOptionalArgNameStruct, obj.Type.String(), structLookupOptionalArgNameField, structLookupOptionalArgNameOptional, obj.Out.String(), obj.Out.String()))
}

// Init runs some startup code for this function.
func (obj *StructLookupOptionalFunc) Init(init *interfaces.Init) error {
	obj.init = init
	return nil
}

// Stream returns the changing values that this func has over time.
func (obj *StructLookupOptionalFunc) Stream(ctx context.Context) error {
	defer close(obj.init.Output) // the sender closes
	for {
		select {
		case input, ok := <-obj.init.Input:
			if !ok {
				return nil // can't output any more
			}
			//if err := input.Type().Cmp(obj.Info().Sig.Input); err != nil {
			//	return errwrap.Wrapf(err, "wrong function input")
			//}

			if obj.last != nil && input.Cmp(obj.last) == nil {
				continue // value didn't change, skip it
			}
			obj.last = input // store for next

			st := (input.Struct()[structLookupOptionalArgNameStruct]).(*types.StructValue)
			field := input.Struct()[structLookupOptionalArgNameField].Str()
			optional := input.Struct()[structLookupOptionalArgNameOptional]

			if field == "" {
				return fmt.Errorf("received empty field")
			}
			if obj.field == "" {
				obj.field = field // store first field
			}
			if field != obj.field {
				return fmt.Errorf("input field changed from: `%s`, to: `%s`", obj.field, field)
			}

			// We know the result of this lookup statically at
			// compile time, but for simplicity we check each time
			// here anyways. Maybe one day there will be a fancy
			// reason why this might vary over time.
			var result types.Value
			val, exists := st.Lookup(field)
			if exists {
				result = val
			} else {
				result = optional
			}

			// if previous input was `2 + 4`, but now it
			// changed to `1 + 5`, the result is still the
			// same, so we can skip sending an update...
			if obj.result != nil && result.Cmp(obj.result) == nil {
				continue // result didn't change
			}
			obj.result = result // store new result

		case <-ctx.Done():
			return nil
		}

		select {
		case obj.init.Output <- obj.result: // send
		case <-ctx.Done():
			return nil
		}
	}
}
