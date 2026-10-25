import Lean
import M2Extract.Canon

/-!
# Extraction records

`extract` makes one record for each bound declaration (spec 07.4). The record gives the canonical type and value, the axioms, the constants and the statement as Lean prints it. The toolchain computes the hashes from the canonical forms.
-/

namespace M2Extract

open Lean Meta

/-- `isNative ax` is true when `ax` is an auxiliary axiom of `native_decide`. Lean adds one such axiom for each use, with `_native` in its name. -/
def isNative (ax : Name) : Bool :=
  ax.components.contains `_native

/-- The constants of a declaration: its type, its value and, for an inductive type, its constructors. -/
def consts (ci : ConstantInfo) : Array Name :=
  let cs := ci.type.getUsedConstants ++ (ci.value? (allowOpaque := true) |>.map Expr.getUsedConstants |>.getD #[])
  match ci with
  | .inductInfo v => cs ++ v.ctors.toArray
  | _ => cs

/-- `boundUses env bound d` gives the bound declarations that `d` reaches through declarations that are not bound. The walk stops at each bound declaration. -/
partial def boundUses (env : Environment) (bound : NameSet) (d : Name) : Array Name := Id.run do
  let mut seen : NameSet := ({} : NameSet).insert d
  let mut out : NameSet := {}
  let mut stack : Array Name := (env.find? d).map consts |>.getD #[]
  while h : stack.size > 0 do
    let c := stack.back
    stack := stack.pop
    if seen.contains c then continue
    seen := seen.insert c
    if bound.contains c then
      out := out.insert c
      continue
    if let some ci := env.find? c then
      stack := stack ++ consts ci
  return out.toArray.qsort Name.quickLt

def sorted (ns : Array Name) : Array Name :=
  (ns.qsort fun a b => a.toString < b.toString).toList.eraseDups.toArray

def names (ns : Array Name) : Json :=
  Json.arr (ns.map fun n => Json.str n.toString)

/-- `pp opts e` prints `e` with the options `opts`. -/
def pp (opts : Options) (e : Expr) : MetaM String :=
  withOptions (fun _ => opts) do return toString (← ppExpr e)

def kind : ConstantInfo → String
  | .axiomInfo _ => "axiom"
  | .defnInfo _ => "def"
  | .thmInfo _ => "theorem"
  | .opaqueInfo _ => "opaque"
  | .quotInfo _ => "quot"
  | .inductInfo _ => "inductive"
  | .ctorInfo _ => "constructor"
  | .recInfo _ => "recursor"

/-- `record bound d` is the extraction record of `d`. -/
def record (bound : NameSet) (d : Name) : MetaM Json := do
  let env ← getEnv
  let some ci := env.find? d
    | return Json.mkObj [("decl", d.toString), ("error", "unknown declaration")]
  let params := ci.levelParams
  let axioms := sorted (← collectAxioms d)
  let module := match env.getModuleIdxFor? d with
    | some i => env.header.moduleNames[i.toNat]!.toString
    | none => ""
  let value := match ci.value? (allowOpaque := true) with
    | some v => Json.str (canon params v)
    | none => Json.null
  let usesDecls := match ci.value? (allowOpaque := true) with
    | some v => sorted v.getUsedConstants
    | none => #[]
  let plain ← pp {} ci.type
  let full ← pp (({} : Options).setBool `pp.all true) ci.type
  return Json.mkObj [
    ("decl", d.toString),
    ("kind", kind ci),
    ("module", module),
    ("level_params", Json.arr (params.toArray.map fun n => Json.str n.toString)),
    ("type", canon params ci.type),
    ("value", value),
    ("statement_pp", plain),
    ("statement_pp_all", full),
    ("axioms", names (axioms.filter (!isNative ·))),
    ("native_decide_axioms", names (axioms.filter isNative)),
    ("uses_decls", names usesDecls),
    ("uses_bound", names (boundUses env bound d)),
    ("uses_constants_in_statement", names (sorted ci.type.getUsedConstants))
  ]

end M2Extract
