import Lean
import M2Extract.Record
import M2Extract.Checks

/-!
# m2extract

`m2extract` answers requests on standard input. Each request is one line of JSON, and each answer is one line of JSON on standard output. The process keeps the environment between requests, so that it loads Mathlib once.

* `{"cmd":"version"}` gives the Lean version and the commit of the toolchain.
* `{"cmd":"import","modules":["Mathlib"]}` loads the modules from the search path.
* `{"cmd":"extract","decls":[...],"bound":[...]}` gives one record for each declaration in `decls`. `bound` is the list of all bound declarations.
* `{"cmd":"axioms","decls":[...]}` gives the axioms of each declaration, as `#print axioms` gives them.
* `{"cmd":"checks","decls":[...],"heartbeats":N}` runs the degenerate-statement checks on each theorem in `decls`. Each tactic has a limit of `N` thousand heartbeats.
* `{"cmd":"bindings"}` gives each declaration that has the attribute `@[m2 "TAG"]` of tamnd/math2, and its tag.

An answer with the field `error` tells that the request failed.
-/

open Lean Meta M2Extract

def runMeta (env : Environment) (x : MetaM Json) : IO Json := do
  let ctx : Core.Context := { fileName := "<m2extract>", fileMap := default, maxHeartbeats := 0 }
  let (j, _) ← (x.run' {} {}).toIO ctx { env }
  return j

def strs (j : Json) (k : String) : Except String (Array Name) := do
  let a ← j.getObjValAs? (Array String) k
  return a.map String.toName

def handle (env? : IO.Ref (Option Environment)) (req : Json) : IO Json := do
  let cmd := (req.getObjValAs? String "cmd").toOption.getD ""
  let needEnv : IO Environment := do
    let some env ← env?.get | throw <| IO.userError "no modules are loaded"
    return env
  match cmd with
  | "version" =>
    return Json.mkObj [("lean", Lean.versionString), ("githash", Lean.githash)]
  | "import" =>
    let ms ← IO.ofExcept (strs req "modules")
    let env ← importModules (ms.map fun m => { module := m }) {} (loadExts := true)
    env?.set env
    return Json.mkObj [("modules", env.header.moduleNames.size), ("constants", toJson (env.constants.fold (fun (n : Nat) _ _ => n + 1) 0))]
  | "extract" =>
    let env ← needEnv
    let ds ← IO.ofExcept (strs req "decls")
    let bound := (← IO.ofExcept (strs req "bound")).foldl NameSet.insert {}
    runMeta env do
      return Json.mkObj [("records", Json.arr (← ds.mapM (record bound)))]
  | "bindings" =>
    let env ← needEnv
    let bs ← unsafe bindings env
    return Json.mkObj [("bindings", Json.arr (bs.map fun b => Json.mkObj [("decl", b.decl.toString), ("tag", b.tag)]))]
  | "checks" =>
    let env ← needEnv
    let ds ← IO.ofExcept (strs req "decls")
    let hb := (req.getObjValAs? Nat "heartbeats").toOption.getD 10000
    runMeta env do
      return Json.mkObj [("checks", Json.arr (← ds.mapM (checks hb)))]
  | "axioms" =>
    let env ← needEnv
    let ds ← IO.ofExcept (strs req "decls")
    runMeta env do
      let mut out := #[]
      for d in ds do
        if env.contains d then
          out := out.push (d.toString, names (sorted (← collectAxioms d)))
        else
          out := out.push (d.toString, Json.null)
      return Json.mkObj [("axioms", Json.mkObj out.toList)]
  | c => throw <| IO.userError s!"unknown command {c.quote}"

unsafe def main : IO UInt32 := do
  initSearchPath (← findSysroot)
  enableInitializersExecution
  let stdin ← IO.getStdin
  let stdout ← IO.getStdout
  let env? ← IO.mkRef none
  repeat
    let line ← stdin.getLine
    if line.isEmpty then break
    if line.all Char.isWhitespace then continue
    let resp ← try
        let req ← IO.ofExcept (Json.parse line)
        handle env? req
      catch e => pure (Json.mkObj [("error", toString e)])
    stdout.putStrLn resp.compress
    stdout.flush
  return 0
