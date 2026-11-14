==============
Document Title
==============
----------
A Subtitle
----------

:Author: Jane Doe
:Authors: A. One; B. Two; C. Three
:Organization: ACME
:Address: 1 Main St.
          Springfield
:Contact: jane@example.org
:Version: 1.2
:Revision: $Revision: 42 $
:Status: Draft
:Date: $Date: 2024-01-02 03:04:05 +0000 (Tue, 02 Jan 2024) $
:Copyright: Public domain.
:Custom Field: custom *value*
:Dedication: To someone.
:Abstract: Short abstract
    over two lines.

.. meta::
   :description: A test document
   :keywords: rst, test
   :http-equiv=Content-Language: en

.. contents:: Table of Contents
   :depth: 2

Section One
===========

Paragraph with *emphasis*, **strong**, ``literal``, `interpreted`,
:sub:`sub` and :sup:`sup` text, a reference_, an `anonymous one`__, a
footnote [1]_, an auto footnote [#]_, labeled [#lbl]_, symbol [*]_ and a
citation [CIT2002]_.  Inline _`target` here; |subst| too.  Standalone
https://example.org/x?y=1#z and mailto:me@example.org, me@example.org.

.. _reference: https://example.org/
__ https://example.org/anon
.. |subst| replace:: replaced *text*
.. [1] Manual footnote.
.. [#] Auto footnote.
.. [#lbl] Labeled auto footnote, see [#lbl]_.
.. [*] Symbol footnote.
.. [CIT2002] A citation.

Subsection
----------

- bullet one
- bullet two

  - nested a
  - nested b

1. first
2. second

   a) alpha
   b) beta

#. auto
#. auto 2

(iv) roman four
(v) roman five

Term
    Definition.
Term 2 : classifier : another
    Definition 2.

    Second paragraph.

-a         option a
-b FILE    option b with argument
--long     long option
--very-long-option=VALUE  description on
           two lines
/V         DOS style

| Line block
|   indented line
| back

    Block quote with attribution.

    -- Somebody Famous

>>> print("doctest")
doctest

Literal block::

    literal  <text> & more
      indented

Quoted literal::

> quoted
> literal

Section Two
===========

+-------+-------+-------+
| Head  | Grid  | Table |
+=======+=======+=======+
| a     | spans two     |
+-------+-------+-------+
| rows  | b     | c     |
+       +-------+-------+
|       | d     | e     |
+-------+-------+-------+

=====  =====  ======
  Inputs      Output
------------  ------
  A      B    A or B
=====  =====  ======
False  False  False
True   False  True
=====  =====  ======

----

.. note:: A note.

.. warning::
   A warning with *markup*.

.. admonition:: Custom Admonition
   :class: special

   Body.

.. image:: picture.png
   :alt: A picture
   :width: 200px
   :align: center

.. figure:: picture.png
   :figwidth: 50%
   :figclass: fancy

   The caption.

   The legend.

.. topic:: Topic Title

   Topic body.

.. sidebar:: Sidebar Title
   :subtitle: Sidebar Subtitle

   Sidebar body.

.. rubric:: A Rubric

.. epigraph::

   An epigraph.

   -- Author

.. compound::

   First part.

   ::

       literal in compound

.. container:: custom

   In container.

.. parsed-literal::

   parsed *literal* with ``markup``

.. code:: python

   def f(x):
       return x

.. class:: classy

Paragraph with class.

.. header:: Header text.
.. footer:: Footer text.

.. comment here
   continues

.. target-notes::
