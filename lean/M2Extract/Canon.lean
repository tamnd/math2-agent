import Lean

/-!
# Canonical form of an expression

The canonical form is the input of the type hash and the value hash (spec 07.4). Two expressions have the same canonical form when they are equal after these changes:

* A universe parameter gets the name of its position in the declaration, `u0`, `u1` and so on.
* A binder name is removed. The binder kind stays, thus an instance argument stays an instance argument.
* Metadata is removed.

The form is a list of lines. Each line is one node of the expression. A line refers to an earlier line by its number, starting at 0. A subexpression that occurs more than once has one line. Thus the size of the form is linear in the size of the expression in memory.
-/

namespace M2Extract

open Lean

/-- `Key` compares expressions by structure, with binder names and binder kinds. -/
structure Key where
  e : Expr

instance : BEq Key where
  beq a b := a.e.equal b.e

instance : Hashable Key where
  hash k := k.e.hash

/-- The canonical text of a level. `params` gives the position of each universe parameter. -/
partial def level (params : List Name) : Level → String
  | .zero => "0"
  | .succ l => s!"s({level params l})"
  | .max a b => s!"m({level params a},{level params b})"
  | .imax a b => s!"i({level params a},{level params b})"
  | .param n => match params.idxOf? n with
    | some i => s!"u{i}"
    | none => s!"?{n}"
  | .mvar _ => "?"

def binder : BinderInfo → String
  | .default => "d"
  | .implicit => "i"
  | .strictImplicit => "s"
  | .instImplicit => "c"

structure State where
  lines : Array String := #[]
  seen : Std.HashMap Key Nat := {}

abbrev M := ReaderT (List Name) (StateM State)

partial def node (e : Expr) : M Nat := do
  if let .mdata _ b := e then
    return ← node b
  if let some i := (← get).seen[Key.mk e]? then
    return i
  let params ← read
  let line ← match e with
    | .bvar i => pure s!"b {i}"
    | .sort l => pure s!"s {level params l}"
    | .const n ls => pure <| " ".intercalate (s!"c {n}" :: ls.map (level params))
    | .app f a => do pure s!"a {← node f} {← node a}"
    | .lam _ t b bi => do pure s!"l {binder bi} {← node t} {← node b}"
    | .forallE _ t b bi => do pure s!"p {binder bi} {← node t} {← node b}"
    | .letE _ t v b nondep => do pure s!"t {if nondep then 1 else 0} {← node t} {← node v} {← node b}"
    | .lit (.natVal n) => pure s!"n {n}"
    | .lit (.strVal s) => pure s!"q {(Json.str s).compress}"
    | .proj s i b => do pure s!"j {s} {i} {← node b}"
    | .fvar _ => pure "?f"
    | .mvar _ => pure "?m"
    | .mdata _ b => pure s!"{← node b}"
  modifyGet fun s => (s.lines.size, { s with lines := s.lines.push line, seen := s.seen.insert (Key.mk e) s.lines.size })

/-- `canon params e` is the canonical form of `e`, one node on each line. `params` is the list of universe parameters of the declaration. -/
def canon (params : List Name) (e : Expr) : String :=
  let (_, s) := (node e).run params |>.run {}
  "\n".intercalate s.lines.toList ++ "\n"

end M2Extract
