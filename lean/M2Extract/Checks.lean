import Lean

/-!
# Degenerate-statement checks

`checks` runs the checks 4, 5 and 6 of spec 10.4 on a statement. A statement is a theorem, or a definition of a proposition, which is how the library states a result that has no proof yet: rule F06 refuses `sorry` in the library. Automation tries a short list of tactics, each with a limit of heartbeats. The check records the first tactic that closes the goal.

* `vacuous`: a tactic proves `False` from the hypotheses. The check skips a statement without a hypothesis that is a proposition, and a statement whose conclusion is `False`.
* `trivial`: a tactic proves the statement, and the proof does not use the declaration itself.
* `refuted`: a tactic proves the negation of the statement.

`implies` compares two statements for the checks 7 and 8. It tries to prove that the first statement implies the second, and that the first statement implies the negation of the second.

The parser reads each tactic at run time. Thus a tactic of Mathlib, for example `aesop`, works when the session loaded Mathlib, and it fails quietly when the session did not.
-/

namespace M2Extract

open Lean Meta Elab

/-- The tactics that try to prove `False` from the hypotheses. -/
def vacuousTactics : List String := ["contradiction", "omega", "simp_all", "grind", "aesop"]

/-- The tactics that try to prove the statement. -/
def trivialTactics : List String := ["rfl", "trivial", "decide", "simp", "grind", "aesop"]

/-- The tactics that try to prove the negation. -/
def refutedTactics : List String := ["decide", "simp", "grind"]

/-- `attempt goal tac heartbeats` reports whether the tactic `tac` closes `goal` within `heartbeats` thousand heartbeats. The proof must not use a constant in `avoid`. The state does not change. -/
def attempt (goal : Expr) (tac : String) (heartbeats : Nat) (avoid : List Name := []) : MetaM Bool := do
  let .ok stx := Parser.runParserCategory (← getEnv) `tactic tac | return false
  let s ← saveState
  let ok ← tryCatchRuntimeEx
    (withTheReader Core.Context (fun ctx => { ctx with maxHeartbeats := heartbeats * 1000, fileMap := FileMap.ofString tac }) do
      withCurrHeartbeats do
        let mvar ← mkFreshExprMVar goal
        let rest ← Term.TermElabM.run' (Term.withoutErrToSorry (Tactic.run mvar.mvarId! (Tactic.evalTactic stx)))
        let proof ← instantiateMVars mvar
        return rest.isEmpty && !proof.hasSorry && !proof.hasMVar && avoid.all (!proof.getUsedConstants.contains ·))
    (fun _ => pure false)
  s.restore
  return ok

/-- `first goal tacs heartbeats avoid` is the first tactic in `tacs` that closes `goal`. -/
def first (goal : Expr) (tacs : List String) (heartbeats : Nat) (avoid : List Name := []) : MetaM (Option String) := do
  for t in tacs do
    if ← attempt goal t heartbeats avoid then
      return some t
  return none

def optStr : Option String → Json
  | some s => Json.str s
  | none => Json.null

/-- `statement ci` is the statement of a declaration: the type of a theorem, or the value of a definition whose type is `Prop`. Another declaration has no statement. -/
def statement : ConstantInfo → Option Expr
  | .thmInfo v => some v.type
  | .defnInfo v => if v.type.isProp then some v.value else none
  | _ => none

/-- `checks heartbeats d` is the result of the checks on the statement `d`. A declaration that is not a statement has an error. -/
def checks (heartbeats : Nat) (d : Name) : MetaM Json := do
  let some ci := (← getEnv).find? d
    | return Json.mkObj [("decl", d.toString), ("error", "unknown declaration")]
  let some stmt := statement ci
    | return Json.mkObj [("decl", d.toString), ("error", "not a statement")]
  let vacuous ← forallTelescope stmt fun xs concl => do
    if concl.isConstOf ``False then return none
    let props ← xs.filterM fun x => return (← inferType (← inferType x)).isProp
    if props.isEmpty then return none
    first (mkConst ``False) vacuousTactics heartbeats
  let trivial ← first stmt trivialTactics heartbeats [d]
  let refuted ← first (mkNot stmt) refutedTactics heartbeats
  return Json.mkObj [
    ("decl", d.toString),
    ("vacuous", optStr vacuous),
    ("trivial", optStr trivial),
    ("refuted", optStr refuted)
  ]

/-- The tactics that try to prove `A → B` for two statements `A` and `B`. -/
def impliesTactics : List String := ["(intro h; exact h)", "(intro h; simpa using h)", "(intro h; grind)", "(intro h; aesop)"]

/-- The tactics that try to prove `A → ¬B`. -/
def contradictsTactics : List String := ["(intro h hb; simp_all)", "(intro h hb; grind)", "(intro h hb; aesop)"]

/-- `implies heartbeats a b` compares the statements `a` and `b`. The field `implies` is the first tactic that proves that the statement of `a` implies the statement of `b`, and the field `contradicts` is the first tactic that proves that it implies the negation. The proofs must not use `a` or `b`. -/
def implies (heartbeats : Nat) (a b : Name) : MetaM Json := do
  let base := [("from", Json.str a.toString), ("to", Json.str b.toString)]
  let env ← getEnv
  let (some ca, some cb) := (env.find? a, env.find? b)
    | return Json.mkObj (base ++ [("error", Json.str "unknown declaration")])
  let (some sa, some sb) := (statement ca, statement cb)
    | return Json.mkObj (base ++ [("error", Json.str "not a statement")])
  let imp ← first (← mkArrow sa sb) impliesTactics heartbeats [a, b]
  let con ← first (← mkArrow sa (mkNot sb)) contradictsTactics heartbeats [a, b]
  return Json.mkObj (base ++ [("implies", optStr imp), ("contradicts", optStr con)])

end M2Extract
