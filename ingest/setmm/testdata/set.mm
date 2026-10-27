$( A small database in the form of set.mm, for the tests of the adapter.
   The text of each comment is ours. It is not from set.mm. $)

$( $t
  htmldef "->" as " &rarr; ";
$)

$c ( ) -> wff |- $.
$v ph ps ch $.
wph $f wff ph $.
wps $f wff ps $.
wch $f wff ch $.

$(
#*#*#*#*#*#*#*#*#*#*#*#*#*#*#*#*#*#*#*#*#*#*#*#*#*#*#*#*#*#*#*#*#*#*#*#*#*#*#*#
  First steps in logic
#*#*#*#*#*#*#*#*#*#*#*#*#*#*#*#*#*#*#*#*#*#*#*#*#*#*#*#*#*#*#*#*#*#*#*#*#*#*#*#

  This header opens a section. Its text is not the text of a statement.
$)

  $( An implication of two formulas is a formula. $)
  wi $a wff ( ph -> ps ) $.

  $( The first axiom: a true formula follows from any formula.
     (Contributed by A. Tester, 1-Jan-2020.) $)
  ax-1 $a |- ( ph -> ( ps -> ph ) ) $.

  ${
    $( The first premise of the rule. $)
    min $e |- ph $.
    maj $e |- ( ph -> ps ) $.
    $( The rule of detachment: from a formula and an implication from it,
       infer the consequent. (Contributed by A. Tester, 2-Jan-2020.) $)
    ax-mp $a |- ps $.
  $}

  $( Every formula implies itself. (Contributed by B. Sample, 3-Feb-2021.)
     (New usage is discouraged.) $)
  id $p |- ( ph -> ph ) $=
    ( wi ax-1 ax-mp ) AABZA? $.

  thm-nocomment $p |- ( ps -> ps ) $= ? $.

$(
=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=
  Words for formulas
=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=
$)

  $( A name for the formula that a formula implies itself. $)
  df-self $a |- ( ch -> ch ) $.
