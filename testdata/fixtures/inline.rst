Inline markup corner cases
==========================

*a* **b** ``c`` `d` |e| f_ g__ [1]_ [#]_ [*]_ [x]_ _`h`

"*quoted*" '*quoted*' (*paren*) [*bracket*] {*brace*} <*angle*>
*not emphasis * and * not either*, 2*3*4, a*b, *a*b, a\ *b*\ c.

**strong**text and text**strong**, ``lit``eral, x``lit``.

Escapes: \*not emphasis\*, \\backslash, \`not interpreted\`, a\ b, C:\\dir.

`phrase reference`_, `phrase <http://example.org/a b>`_, `<http://bare.example.org>`_,
`alias <phrase reference_>`_, `anon embedded <http://x.org>`__, `with \<escaped\>`_.

.. _phrase reference: http://phrase.example.org
.. _with <escaped>: http://esc.example.org

:emphasis:`role`, `suffix role`:strong:, :literal:`lit\\eral`, :code:`x = 1`,
:math:`a^2`, :pep:`8`, :PEP:`287`, :rfc:`2822`, :rfc:`2822#section-3`, :title:`T`,
:ab:`abbr`, :ac:`ACR`, :sub:`s`, :sup:`s`, :pep:`x`, :rfc:`0`.

URIs: http://www.example.org, <http://example.org/path>, (https://example.org/),
ftp://ftp.example.org/file.txt; news:comp.lang.python, urn:isbn:0-486-27557-4,
http://example.org/a_(b), http://example.org/x., foo:bar, user@example.org,
mailto:user@example.org?subject=hi, http://example.org/\*escaped, www.example.org.

Footnote refs: text [1]_ and[2]_ and [3]_.

.. [1] one
.. [2] two
.. [3] three
.. [#] auto
.. [*] sym
.. [x] cit

Substitutions: |e|, |e|_, |e|__, |img|, |date-ish| and |uni|.

.. |e| replace:: *E* text
.. _e: http://e.example.org
__ http://anon.example.org
__ http://anon2.example.org
.. |img| image:: i.png
   :alt: the image
.. |date-ish| replace:: 2024
.. |uni| unicode:: U+00A9 .. copyright
   :trim:

Unicode: «*guillemets*» „*quotes*“ ‘*single*’ —*dash*— ¿*que*? 日本語*強調*テキスト,
日本語 *強調* テキスト, naïve café `ref with ünïcode`_.

.. _ref with ünïcode: http://unicode.example.org

Interpreted `default role`, ``literal with `backquote` inside``, ```triple```,
`` ` ``, **\***, *\**, ``\``.

_`inline target` and `inline target`_ reference. _`multi word target`.

Long: aaaaaaaaaa*bbbbbbbbbb, __init__, snake_case_name, a_b_, _private, x__, y_.

.. _x: http://x.example.org
.. _y: http://y.example.org
.. _a_b: http://ab.example.org
