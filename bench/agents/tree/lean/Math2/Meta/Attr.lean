module

public meta import Lean.Elab.Command

/-!
# The m2 attribute

`@[m2 "TAG"]` marks a declaration as the formal side of a binding (spec 07.2). `TAG` is the tag of the node, or the provisional id of a new node. Rule F01 of the audit checks that the front matter of the node has the same binding.

A node can also bind to a Mathlib declaration. Math2 cannot add an attribute to Mathlib, thus only the front matter and `registers/bindings.reg` record such a binding.

M2Extract in tamnd/math2-agent reads the bindings from `bindingExt`. If you change the structure `Binding`, change M2Extract in the same release.

This file and `Math2/Meta/Match.lean` are the only metaprogramming in the package.
-/

public meta section

open Lean Elab

namespace Math2.Meta

/-- A binding on the Lean side: a declaration and the tag of its node. -/
structure Binding where
  /-- The declaration that has the attribute. -/
  decl : Name
  /-- The tag or the provisional id of the node. -/
  tag : String

/-- The characters of Crockford base32, in order. -/
def crockford : List Char := "0123456789ABCDEFGHJKMNPQRSTVWXYZ".toList

/-- `isTag s` is true when `s` is a tag, for example `7K3QX2`, or a provisional id: `~` and 8 lowercase hex digits. -/
def isTag (s : String) : Bool :=
  match s.toList with
  | '~' :: rest => rest.length == 8 && rest.all fun c => c.isDigit || ('a' ≤ c && c ≤ 'f')
  | cs => cs.length == 6 && cs.all (crockford.contains ·)

/-- The bindings of each module. The state of an imported module is empty. Use `bindings` to read all bindings. -/
initialize bindingExt : SimplePersistentEnvExtension Binding (Array Binding) ←
  registerSimplePersistentEnvExtension {
    addImportedFn := fun _ => #[]
    addEntryFn := Array.push
  }

/-- The bindings of the imported modules and of the current module, in order of module. -/
def bindings (env : Environment) : Array Binding := Id.run do
  let mut out := #[]
  for i in [:env.header.moduleNames.size] do
    out := out ++ bindingExt.getModuleEntries env i
  return out ++ bindingExt.getState env

/-- `@[m2 "TAG"]` binds the declaration to the node with the tag `TAG`. -/
syntax (name := m2) "m2 " str : attr

initialize registerBuiltinAttribute {
  name := `m2
  descr := "Bind the declaration to the math2 node with the given tag."
  add := fun decl stx kind => do
    unless kind == .global do
      throwError "The m2 attribute must be global."
    let `(attr| m2 $tag:str) := stx | throwUnsupportedSyntax
    let t := tag.getString
    unless isTag t do
      throwError "{t.quote} is not a tag or a provisional id."
    if let some b := (bindings (← getEnv)).find? (·.decl == decl) then
      throwError "{.ofConstName decl} is already bound to {b.tag}."
    modifyEnv (bindingExt.addEntry · { decl, tag := t })
}

end Math2.Meta
