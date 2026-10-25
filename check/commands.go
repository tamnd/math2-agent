package check

import "strings"

// The lists in this file are static. They hold the TeX commands and environments that both KaTeX and MathJax 3 render with their default packages. A command that is in one renderer only is a B01 error, because the site renders with KaTeX and an agent can render with MathJax. A math2 macro goes in macros/m2.sty and macros/m2.katex.json, and not in these lists.

// commands are the control words that both renderers know.
var commands = words(`
alpha beta gamma delta epsilon varepsilon zeta eta theta vartheta iota kappa varkappa lambda mu nu xi omicron pi varpi rho varrho sigma varsigma tau upsilon phi varphi chi psi omega digamma
Gamma Delta Theta Lambda Xi Pi Sigma Upsilon Phi Psi Omega varGamma varDelta varTheta varLambda varXi varPi varSigma varUpsilon varPhi varPsi varOmega
aleph beth gimel daleth ell hbar hslash imath jmath wp Re Im partial nabla infty emptyset varnothing complement eth Finv Game mho Bbbk
pm mp times div cdot ast star circ bullet oplus ominus otimes oslash odot bigcirc dagger ddagger amalg cap cup uplus sqcap sqcup vee wedge lor land setminus smallsetminus wr diamond
triangleleft triangleright bigtriangleup bigtriangledown lhd rhd unlhd unrhd boxplus boxminus boxtimes boxdot dotplus divideontimes ltimes rtimes leftthreetimes rightthreetimes
curlywedge curlyvee barwedge doublebarwedge Cap Cup centerdot intercal circleddash circledast circledcirc mod bmod pmod pod
leq le geq ge neq ne equiv approx approxeq cong simeq sim propto prec succ preceq succeq ll gg lll ggg subset supset subseteq supseteq subsetneq supsetneq subseteqq supseteqq subsetneqq supsetneqq
nsubseteq nsupseteq nsubseteqq nsupseteqq varsubsetneq varsupsetneq varsubsetneqq varsupsetneqq Subset Supset sqsubset sqsupset sqsubseteq sqsupseteq
in ni notin owns mid nmid parallel nparallel perp vdash dashv models smile frown asymp doteq bowtie Join lessgtr gtrless leqslant geqslant nleqslant ngeqslant lesssim gtrsim lessapprox gtrapprox
nless ngtr nleq ngeq nleqq ngeqq lneq gneq lneqq gneqq leqq geqq ncong nsim nprec nsucc npreceq nsucceq triangleq trianglelefteq trianglerighteq ntriangleleft ntriangleright ntrianglelefteq ntrianglerighteq
vDash Vdash Vvdash nvdash nvDash nVdash nVDash therefore because between pitchfork backsim backsimeq eqcirc circeq bumpeq Bumpeq doteqdot fallingdotseq risingdotseq thicksim thickapprox
precsim succsim precapprox succapprox precnsim succnsim precnapprox succnapprox lnsim gnsim lnapprox gnapprox eqslantless eqslantgtr lesseqgtr gtreqless lesseqqgtr gtreqqless lessdot gtrdot
varpropto blacktriangleleft blacktriangleright shortmid shortparallel nshortmid nshortparallel colon not
to gets leftarrow rightarrow Leftarrow Rightarrow leftrightarrow Leftrightarrow longleftarrow longrightarrow Longleftarrow Longrightarrow longleftrightarrow Longleftrightarrow
mapsto longmapsto hookleftarrow hookrightarrow uparrow downarrow Uparrow Downarrow updownarrow Updownarrow nearrow searrow swarrow nwarrow
leftharpoonup leftharpoondown rightharpoonup rightharpoondown rightleftharpoons leftrightharpoons upharpoonleft upharpoonright downharpoonleft downharpoonright
iff implies impliedby twoheadrightarrow twoheadleftarrow rightarrowtail leftarrowtail leadsto rightsquigarrow leftrightsquigarrow curvearrowleft curvearrowright circlearrowleft circlearrowright
Lsh Rsh nleftarrow nrightarrow nLeftarrow nRightarrow nleftrightarrow nLeftrightarrow dashrightarrow dashleftarrow leftleftarrows rightrightarrows leftrightarrows rightleftarrows
upuparrows downdownarrows Lleftarrow Rrightarrow multimap looparrowleft looparrowright restriction xrightarrow xleftarrow
sum prod coprod int iint iiint oint bigcup bigcap bigvee bigwedge bigoplus bigotimes bigodot biguplus bigsqcup smallint intop
arccos arcsin arctan arg cos cosh cot coth csc deg det dim exp gcd hom inf ker lg lim liminf limsup ln log max min Pr sec sin sinh sup tan tanh
injlim projlim varliminf varlimsup varinjlim varprojlim operatorname
left right middle big Big bigg Bigg bigl bigr Bigl Bigr biggl biggr Biggl Biggr bigm Bigm biggm Biggm
langle rangle lvert rvert lVert rVert vert Vert lfloor rfloor lceil rceil lbrace rbrace lbrack rbrack backslash lgroup rgroup lmoustache rmoustache ulcorner urcorner llcorner lrcorner
hat widehat tilde widetilde bar overline underline vec dot ddot dddot acute grave breve check mathring
overrightarrow overleftarrow overleftrightarrow underrightarrow underleftarrow underleftrightarrow overbrace underbrace overset underset stackrel
frac dfrac tfrac cfrac binom dbinom tbinom sqrt over choose atop
mathbb mathbf mathit mathrm mathsf mathtt mathcal mathfrak mathscr boldsymbol
text textbf textit textrm textsf texttt textnormal mbox hbox rm bf it sf tt cal
displaystyle textstyle scriptstyle scriptscriptstyle
quad qquad enspace thinspace medspace thickspace negthinspace negmedspace negthickspace hspace kern mkern mskip hskip phantom hphantom vphantom smash
dots ldots cdots vdots ddots dotsc dotsb dotsm dotsi dotso
forall exists nexists neg lnot top bot angle measuredangle sphericalangle triangle square blacksquare lozenge blacklozenge bigstar
diamondsuit heartsuit clubsuit spadesuit flat natural sharp checkmark prime backprime surd S circledS diagup diagdown
substack begin end hline cr tag notag
color textcolor boxed cancel bcancel xcancel
limits nolimits displaylimits mathop mathbin mathrel mathord mathopen mathclose mathpunct mathinner
`)

// controlSymbols are the one-character commands that both renderers know, such as \, and \{.
const controlSymbols = `\,;:!|{}#$%&_ >`

// environments are the environments that both renderers know.
var environments = words(`
matrix pmatrix bmatrix Bmatrix vmatrix Vmatrix smallmatrix cases aligned alignedat gathered array subarray split
align align* gather gather* equation equation* CD
`)

// mathJaxOnly and katexOnly are the commands that one renderer knows and the other does not. They get a clear message.
var (
	mathJaxOnly = words(`bbox class cssId style require unicode mmlToken`)
	katexOnly   = words(`htmlClass htmlId htmlStyle htmlData includegraphics`)
)

func words(s string) map[string]bool {
	m := map[string]bool{}
	for _, w := range strings.Fields(s) {
		m[w] = true
	}
	return m
}
