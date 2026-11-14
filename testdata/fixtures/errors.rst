Title
====

Short underline above.

.. unknown-directive:: argument
   :option: value

   content

Unknown role :foo:`bar` and undefined ref_ and |undefined| subst.

*unterminated emphasis and ``unterminated literal

Footnote ref [5]_ missing, auto [#]_ missing, anon__ missing.

.. _dup: http://a/
.. _dup: http://b/

dup_

 Unexpected indentation
after block quote
  again.

- list
without blank line

1. enumerated
text

.. note::

.. image::

.. figure:: x.png
   :scale: abc

.. include:: does-not-exist.rst

.. csv-table:: bad
   :header-rows: 5

   a,b

+-----+
| bad |
+----

=====  ====
bad table
=====

Section
-------

Level skip
~~~~~~~~~~

Another
=======

Bad
~~~

.. |empty| replace::

.. [#] Unreferenced footnote

.. _unused target:

`embedded <http://x`_ and `bad <uri> ref`_.

::

.. class::

.. role:: custom(nonexistent)

.. default-role:: nonexistent

.. table::

   not a table

.. list-table::

   * - a
   * - b
     - c
