Tables
======

.. table:: Titled table
   :widths: 10 20 30
   :align: center
   :width: 80%
   :name: tbl

   ===  ===  ===
   a    b    c
   ===  ===  ===
   1    2    3
   ===  ===  ===

.. table::
   :widths: auto

   +---+---+
   | a | b |
   +---+---+

.. csv-table:: CSV
   :header: "Name", "Value, with comma", Note
   :widths: 15, 10, 30
   :stub-columns: 1

   "Alpha", 1, "multi
   line"
   Beta, 2
   "Quote ""inside""", 3, *markup*

.. csv-table::
   :delim: ;
   :quote: '
   :header-rows: 1

   h1;h2
   'a;b';c

.. csv-table::
   :file: data.csv
   :header-rows: 1

.. list-table:: List table
   :header-rows: 1
   :stub-columns: 1
   :widths: 1 2 3

   * - h1
     - h2
     - h3
   * - a
     - b

       two paragraphs
     - * nested
       * list

Simple table with spans and multi-line cells:

=====  =====  =======
A      B      A and B
-----  -----  -------
first  second third
       line
-----  ------------
x      spanning
=====  ============

==== ====
日本 語語
==== ====
ｆｕ ll
==== ====

+------------+------------+-----------+
| Header 1   | Header 2   | Header 3  |
+============+============+===========+
| body row 1 | column 2   | column 3  |
+------------+------------+-----------+
| body row 2 | Cells may span columns.|
+------------+------------+-----------+
| body row 3 | Cells may  | - Cells   |
+------------+ span rows. | - contain |
| body row 4 |            | - blocks. |
+------------+------------+-----------+

+--+
|  |
+--+
