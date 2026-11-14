Math as HTML (math2html)
========================

Inline: :math:`x^2 + y_1`, :math:`a_i^j`, :math:`x^{2n}_{k+1}`,
:math:`\frac{a}{b}`, :math:`\sqrt[3]{x}`, :math:`\sum_{i=1}^n i`,
:math:`\alpha\beta\Gamma`, :math:`\mathrm{d}x`, :math:`2\mathrm{m}`,
:math:`\hat{x} \bar{ab} \vec{v} \not= \not\in`, :math:`a \le b \ne c`,
:math:`f(x) = \sin x`, :math:`\text{some text}`, :math:`\mathbb{R}^n`,
:math:`\left( \frac{a}{b} \right)`, :math:`\binom{n}{k}`,
:math:`a\,b\;c\quad d`, :math:`x' < y > z`, :math:`100\%`,
:math:`\upalpha`, :math:`\overline{x+y}`, :math:`\underbrace{a+b}`.

Fractions and roots:

.. math:: \frac{1}{1+\frac{1}{x}} \quad \dfrac{a}{b} \quad \tfrac{a}{b}
          \quad \cfrac[l]{a}{b} \quad \nicefrac{1}{2} \quad \sqrt{x}
          \quad \sqrt[n]{1+x}

Big operators with limits:

.. math:: \sum_{i=1}^n i \quad \prod_k a_k \quad \int_0^1 f(x)\,dx \quad
          \sum\limits_{i} \quad \int\nolimits_a^b \quad \lim_{x\to 0} f
          \quad \sum^{n} \quad \max_i x_i

Delimiters:

.. math:: \left( \frac{a}{b} \right) \left[ x \right] \left\{ y \right\}
          \left. \frac{a}{b} \right|_0
          \left\langle a \middle| b \right\rangle
          \left( \frac{\frac{a}{b}}{c} \right)
          \bigl( \Bigl( \biggl( x \biggr) \Bigr) \bigr)

Matrices and arrays:

.. math:: \begin{matrix} a & b \\ c & d \end{matrix}

.. math:: \left( \begin{matrix} a \\ b \\ c \end{matrix} \right)

.. math:: \begin{pmatrix} a & b \\ c & d \end{pmatrix}
          \begin{bmatrix} 1 & 0 \\ 0 & 1 \end{bmatrix}

.. math:: \begin{array}{lcr} a & b & c \\ d & e & f \end{array}

.. math:: \left\{ \begin{array}{l} a \\ b \\ c \\ d \end{array} \right.

Cases:

.. math:: f(x) = \begin{cases} 1 & x > 0 \\ 0 & \text{otherwise} \end{cases}

Aligned environments:

.. math::

   a &= b + c \\
     &= d

.. math:: \begin{align*} a &= b \\ c &= d \end{align*}

.. math:: \begin{eqnarray} a & = & b \end{eqnarray}

.. math:: \begin{gathered} a \\ b \end{gathered}

Two formulas in one directive:

.. math::

   a = b

   c = d

Accents and embellishments:

.. math:: \hat{x} \bar{x} \tilde{x} \vec{x} \dot{x} \ddot{x} \acute{a}
          \grave{a} \check{a} \breve{a} \bar{\alpha} \bar{+} \bar{1}
          \widehat{abc} \overline{x+y} \underline{x} \overbrace{a+b}
          \overrightarrow{AB} \stackrel{def}{=} \overset{!}{=}
          \underset{x}{y}

Fonts and text:

.. math:: \mathrm{d}x \mathbf{v} \mathit{diff} \mathsf{A} \mathtt{code}
          \mathcal{L} \mathbb{R} \mathfrak{g} \mathscr{F}
          \boldsymbol{\alpha} \text{a_b^c \alpha {x}} \textbf{bold}
          \mbox{box} 3\text{kg}

Spacing, colour and boxes:

.. math:: a\,b\:c\;d\quad e\qquad f\ g\!h ~ i \hspace{1em} j
          \color{red}{x} \textcolor{blue}{y} \colorbox{yellow}{z}
          \boxed{x} \fbox{y} \rule{1cm}{2pt} \phantom{x}

Symbols:

.. math:: \to \Rightarrow \mapsto \iff \ldots \cdots \infty \partial
          \nabla \forall \exists \emptyset \hbar \ell \aleph a \cdot b
          \times c \pm d \otimes e \& \% \# \_ \{ \}

Unicode input: :math:`α ≤ β`, :math:`∑ x²`, :math:`é × ñ`.

Problems are reported by math2html on stderr only:

.. math:: \unknowncommand + \foo{bar} + \Upgamma + \begin{unknownenv} a & b
          \end{unknownenv} + { a
