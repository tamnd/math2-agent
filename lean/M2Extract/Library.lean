import Lean
import M2Extract.Canon
import M2Extract.Record

/-!
# Library records

`library` lists the declarations of a library, such as Mathlib, for the record plane (spec 09.2). A library is the set of modules whose first component is its root. The record of a declaration is smaller than the extraction record of a bound declaration: it gives the kind, the module, the canonical type, the constants of the value and the docstring.
-/

namespace M2Extract

open Lean

/-- `libraryDecls env root` gives the declarations of the modules whose first component is `root`, in the order of their names as strings. It leaves out the internal names, such as auxiliary definitions and the names that start with an underscore. -/
def libraryDecls (env : Environment) (root : Name) : Array Name := Id.run do
  let mods := env.header.moduleNames
  let mut out := #[]
  for (n, _) in env.constants.map₁.toList do
    if n.isInternalDetail then continue
    let some i := env.getModuleIdxFor? n | continue
    if mods[i.toNat]!.getRoot == root then
      out := out.push n
  return out.qsort fun a b => a.toString < b.toString

/-- `libraryRecord env d` is the library record of `d`. -/
def libraryRecord (env : Environment) (d : Name) : IO Json := do
  let some ci := env.find? d
    | return Json.mkObj [("decl", d.toString), ("error", "unknown declaration")]
  let module := match env.getModuleIdxFor? d with
    | some i => env.header.moduleNames[i.toNat]!.toString
    | none => ""
  let uses := match ci.value? (allowOpaque := true) with
    | some v => sorted v.getUsedConstants
    | none => #[]
  let doc ← findDocString? env d
  return Json.mkObj [
    ("decl", d.toString),
    ("kind", kind ci),
    ("module", module),
    ("type", canon ci.levelParams ci.type),
    ("uses", names uses),
    ("doc", (doc.map Json.str).getD Json.null)
  ]

end M2Extract
