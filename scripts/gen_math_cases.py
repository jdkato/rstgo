#!/usr/bin/env python3
"""Generate reference results of Docutils' math2html for the Go port.

  python3 scripts/gen_math_cases.py OUT.json [SEED [MUTATIONS]]

Collects the formulas of the documents under testdata/ (math roles and
directives), a list of hand-written constructs and SEED-dependent random
mutations of both, and records for each what
docutils.utils.math.math2html.math2html() returns, what it prints to
stderr, and the exception it raises, if any.  testdata/math2html.json
(used by TestMath2HTML) is the output for seed 1 with 4000 mutations.
"""
import io
import json
import os
import random
import sys

from docutils import core, nodes
from docutils.utils.math import math2html, wrap_math_code, unichar2tex

HAND = r'''
x^2 + y_1
a_i^j
a^j_i
x^{2n}_{k+1}
\frac{a}{b}
\frac{1}{1+\frac{1}{x}}
\dfrac{a}{b} \tfrac{a}{b} \cfrac{a}{b} \cfrac[l]{a}{b}
\binom{n}{k} \dbinom{n}{k} \tbinom{n}{k}
\sqrt{x} \sqrt[3]{x} \sqrt[n]{1+x}
\sum_{i=1}^n i \prod_{k} \int_0^1 f(x)\,dx
\sum\limits_{i=1}^n \int\nolimits_a^b
\lim_{x\to 0} \max_i \sup x
\left( \frac{a}{b} \right) \left[ x \right] \left\{ y \right\}
\left. \frac{a}{b} \right|_0
\left\langle a \middle| b \right\rangle
\left( \frac{\frac{a}{b}}{c} \right)
\bigl( \Bigl( \biggl( \Biggl( x \Biggr) \biggr) \Bigr) \bigr)
\begin{matrix} a & b \\ c & d \end{matrix}
\begin{pmatrix} a & b \\ c & d \end{pmatrix}
\begin{bmatrix} 1 & 0 \\ 0 & 1 \end{bmatrix}
\begin{vmatrix} a & b \\ c & d \end{vmatrix}
\begin{array}{lcr} a & b & c \\ d & e & f \end{array}
\begin{array}[t]{l|r} a & b \end{array}
\begin{cases} 1 & x > 0 \\ 0 & \text{otherwise} \end{cases}
\begin{align} a &= b \\ c &= d \end{align}
\begin{align*} a &= b + c \\ &= d \end{align*}
\begin{eqnarray} a & = & b \end{eqnarray}
\begin{gathered} a \\ b \end{gathered}
\begin{smallmatrix} a & b \\ c & d \end{smallmatrix}
\begin{equation} E = mc^2 \end{equation}
\begin{unknownenv} a & b \end{unknownenv}
\hat{x} \bar{x} \tilde{x} \vec{x} \dot{x} \ddot{x} \acute{a} \grave{a} \check{a} \breve{a}
\hat x \bar{ab} \vec{AB} \widehat{abc} \widetilde{abc} \not= \not\in
\overline{x+y} \underline{x} \overbrace{a+b} \underbrace{a+b}
\overrightarrow{AB} \overleftarrow{AB} \underrightarrow{x}
\mathrm{d}x \mathbf{v} \mathit{diff} \mathsf{A} \mathtt{code} \mathcal{L} \mathbb{R} \mathfrak{g} \mathscr{F}
\boldsymbol{\alpha} \text{some text} \textbf{bold} \textit{it} \mbox{box} \textrm{rm $x$}
\text{a_b^c \alpha {x} % c}
2\mathrm{m} 3\text{kg}
\alpha \beta \gamma \Gamma \Delta \omega \Omega \varepsilon \vartheta \upalpha \Upgamma \uparrow
a \le b \ge c \ne d \approx e \equiv f \in G \subset H
a \cdot b \times c \pm d \otimes e
\to \rightarrow \Rightarrow \mapsto \leftrightarrow \iff
a\,b\:c\;d\quad e\qquad f\ g\!h ~ i
\ldots \cdots \vdots \ddots \dots
\infty \partial \nabla \forall \exists \emptyset \hbar \ell \Re \Im \aleph
f(x) = \sin x + \cos^2 x + \log_2 n + \exp(-x)
\operatorname{foo} \mathop{bar}
\unknowncommand \foo{bar} \\ \& \% \$ \# \_ \{ \}
x' + y'' a < b > c a = b a - b a / b a : b a, b a; b a! a | b a* a@b
%comment
a % comment
100\%
\color{red}{x} \textcolor{blue}{y} \colorbox{yellow}{z} \fcolorbox{red}{blue}{w}
\boxed{x} \fbox{y} \framebox[2cm][l]{z}
\hspace{1em} \vspace{2ex} \rule{1cm}{2pt} \rule[1ex]{1cm}{2pt}
\stackrel{def}{=} \overset{!}{=} \underset{x}{y}
\displaystyle\frac{a}{b} \textstyle x \scriptstyle y \scriptscriptstyle z
\nicefrac{1}{2} \unitfrac{m}{s} \unit[5]{kg} \unitfrac[3]{a}{b}
\href{http://x.org}{link} \url{http://x.org}
\raisebox{1ex}{x} \parbox{3cm}{x}
\label{eq:1} \tag{1} \nonumber
\phantom{x} \hphantom{y} \vphantom{z}
\mathring{a} \dddot{x}
\widehat{x+y}_i^2
x_1^2 x_{12}^{34} {}_a^b
^a_b
_
^
\frac
\frac{a}
\frac12
\sqrt
\bar
\hat
\left(
\left( a
\right)
\begin{array}
\begin
\begin{matrix}
\begin{cases} a
\end{matrix}
{ a
a }
{}
\{ \}
{\}}
\text
\text{
\mathrm
\hspace
\color
[a] (b) [
\sqrt[3
\sqrt[]{x}
a & b
a \\ b
\\
\
$
a $ b
é ñ ü × ÷ ± ∑ ∫ α β
x² ½ ①
\mathbb{ABC}_1
\sum_{i} \sum^{n} \sum_{i}^{n} \sum^{n}_{i} \sum
\int\limits_a^b f \oint_C
a^{b^{c}} a_{b_{c}}
\left( \begin{matrix} a \\ b \\ c \end{matrix} \right)
\left\{ \begin{array}{l} a \\ b \\ c \\ d \end{array} \right.
\left[ \frac{a}{b} \right] \left| \frac{a}{b} \right| \left\| \frac{a}{b} \right\|
\left< x \right>
\bar{\alpha} \bar{+} \bar{1} \bar{\mathrm{x}} \bar{{x}} \bar{\frac{a}{b}} \vec{}
\not\equiv \not{=} \not a
\overset{a} \underset{}{b}
'''.strip('\n').split('\n')

ALPHABET = ['\\', '{', '}', '[', ']', '^', '_', '&', '\\\\', '%', '$', ' ', '\n',
            '(', ')', '|', '.', '*', '=', '<', '>', '+', '-', ',', "'", '~', '!',
            '1', '23', 'a', 'xy', '\\frac', '\\sqrt', '\\left', '\\right',
            '\\begin{', '\\end{', 'matrix}', 'array}', 'cases}', 'align}',
            '\\text', '\\mathrm', '\\bar', '\\hat', '\\sum', '\\int', '\\lim',
            '\\alpha', '\\,', '\\{', '\\}', '\\limits', '\\binom', '\\overset',
            '\\color', '\\hspace', '\\rule', '\\foo', '\\up', '\\not', 'é', '∑',
            '\\middle', '\\big', '\\cfrac', '\\unit', '\\label', '﻿']


def corpus_formulas():
    found = []
    for root in ('testdata/fixtures', 'testdata/docutils'):
        for dp, dn, fn in sorted(os.walk(root)):
            for f in sorted(fn):
                if not f.endswith('.rst'):
                    continue
                path = os.path.join(dp, f)
                src = open(path, encoding='utf-8', newline='').read()
                if 'math' not in src:
                    continue
                try:
                    doc = core.publish_doctree(
                        src, source_path=path, settings_overrides={
                            'warning_stream': io.StringIO(),
                            '_disable_config': True, 'report_level': 5,
                            'halt_level': 5, 'file_insertion_enabled': False})
                except Exception:
                    continue
                for node in doc.findall(lambda n: isinstance(
                        n, (nodes.math, nodes.math_block))):
                    code = node.astext().translate(unichar2tex.uni2tex_table)
                    found.append((code, isinstance(node, nodes.math_block)))
    return found


def mutate(rnd, s):
    for _ in range(rnd.randint(1, 3)):
        op = rnd.randrange(5)
        i = rnd.randint(0, len(s))
        if op == 0:
            s = s[:i] + rnd.choice(ALPHABET) + s[i:]
        elif op == 1 and s:
            j = min(len(s), i + rnd.randint(1, 4))
            s = s[:i] + s[j:]
        elif op == 2:
            s = s[:i]
        elif op == 3 and s:
            j = rnd.randint(0, len(s))
            a, b = sorted((i, j))
            s = s[:a] + s[b:] + s[a:b]
        else:
            j = rnd.randint(0, len(s))
            a, b = sorted((i, j))
            s = s[:a] + s[a:b] + s[a:b] + s[b:]
    return s


def run(code, block):
    err = io.StringIO()
    sys.stderr = err
    crash = ''
    html = ''
    try:
        math2html.DocumentParameters.displaymode = block
        html = math2html.math2html(wrap_math_code(code, block))
    except RecursionError:
        crash = 'RecursionError'
    except Exception as e:
        crash = '%s: %s' % (type(e).__name__, e)
    finally:
        sys.stderr = sys.__stderr__
    return {'code': code, 'block': block, 'html': html,
            'errors': err.getvalue(), 'crash': crash}


def main():
    out = sys.argv[1]
    seed = int(sys.argv[2]) if len(sys.argv) > 2 else 1
    count = int(sys.argv[3]) if len(sys.argv) > 3 else 4000
    rnd = random.Random(seed)
    base = []
    seen = set()
    for item in corpus_formulas() + [(h, b) for h in HAND for b in (False, True)]:
        if item not in seen:
            seen.add(item)
            base.append(item)
    cases = list(base)
    while len(cases) < len(base) + count:
        code, block = rnd.choice(base)
        m = (mutate(rnd, code), block)
        if m not in seen:
            seen.add(m)
            cases.append(m)
    results = [run(code, block) for code, block in cases]
    with open(out, 'w', encoding='utf-8') as f:
        json.dump(results, f, ensure_ascii=False, indent=0)
    print('%d formulas (%d from documents and hand-written), %d crash in Docutils'
          % (len(results), len(base), sum(1 for r in results if r['crash'])))


if __name__ == '__main__':
    main()
