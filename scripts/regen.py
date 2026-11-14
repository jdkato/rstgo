#!/usr/bin/env python3
"""Regenerate the expected outputs under testdata/ with the installed
Docutils (see scripts/regen.sh).

For every fixture testdata/fixtures/NAME.rst this writes

  NAME.pxml   pseudo-XML document tree (rst2pseudoxml)
  NAME.html4  full document of the html4css1 writer (rst2html)
  NAME.html5  full document of the html5_polyglot writer (rst2html5)
  NAME.err    system messages written to stderr (pseudo-XML run)
  NAME.err4 / NAME.err5  stderr of the HTML runs, only if different

and for every suite testdata/suites/NAME.json (a list of small inputs)
testdata/suites/NAME.expected.json with the same data per case.

Unless a fixture has a NAME.json settings file saying otherwise, documents
are generated without a stylesheet (stylesheet="") to keep the corpus small.
A run that halts (SEVERE system message) is recorded as {"halt": true}.
"""
import io
import json
import os
import pickle
import sys

import docutils
from docutils import core
from docutils.parsers.rst import directives, roles
from docutils.utils import SystemMessage

ROOT = os.path.join(os.path.dirname(os.path.abspath(__file__)), '..')
os.chdir(ROOT)

WRITERS = (('pxml', 'pseudoxml'), ('html4', 'html4'), ('html5', 'html5'))

# settings understood in fixture .json files (same names in corpus_test.go)
ALLOWED = {
    'initial_header_level', 'doctitle_xform', 'docinfo_xform',
    'sectsubtitle_xform', 'report_level', 'halt_level',
    'file_insertion_enabled', 'raw_enabled', 'tab_width', 'toc_backlinks',
    'footnote_references', 'sectnum_xform', 'strip_comments', 'generator',
    'source_link', 'source_url', 'pep_references', 'rfc_references',
    'trim_footnote_reference_space', 'math_output', 'cloak_email_addresses',
    'compact_lists', 'compact_field_lists', 'table_style', 'attribution',
    'xml_declaration', 'syntax_highlight', 'character_level_inline_markup',
    'field_name_limit', 'option_limit', 'title', 'section_self_link',
    'line_length_limit', 'strip_classes', 'strip_elements_with_classes',
    'footnote_backlinks', 'embed_stylesheet', 'default_stylesheet',
    'stylesheet', 'image_loading', 'smart_quotes',
}


def run(source, source_path, writer, overrides):
    """Run Docutils in a forked child process, so that no global state
    (role and directive registries, class attributes mutated by the
    writers, ...) leaks from one document into the next -- every result is
    what a fresh `rst2html` process would produce."""
    r, w = os.pipe()
    pid = os.fork()
    if pid == 0:
        os.close(r)
        try:
            result = _run(source, source_path, writer, overrides)
            with os.fdopen(w, 'wb') as f:
                pickle.dump(result, f)
        finally:
            os._exit(0)
    os.close(w)
    with os.fdopen(r, 'rb') as f:
        data = f.read()
    os.waitpid(pid, 0)
    if not data:
        raise SystemExit('Docutils crashed on %s (%s)' % (source_path, writer))
    return pickle.loads(data)


def _run(source, source_path, writer, overrides):
    roles._roles = {}
    directives._directives = {}
    stderr = io.StringIO()
    settings = {
        'warning_stream': stderr,
        'output_encoding': 'utf-8',
        'input_encoding': 'utf-8',
        '_disable_config': True,
        'traceback': True,
        'stylesheet_path': None,
        'stylesheet': None,
    }
    o = dict(overrides)
    if o.pop('default_stylesheet', False):
        del settings['stylesheet_path']
        del settings['stylesheet']
    for k in o:
        if k not in ALLOWED:
            raise SystemExit('unknown setting %r for %s' % (k, source_path))
    settings.update(o)
    if writer == 'pseudoxml':
        for k in ('stylesheet_path', 'stylesheet', 'embed_stylesheet',
                  'math_output', 'initial_header_level',
                  'footnote_references', 'cloak_email_addresses',
                  'compact_lists', 'compact_field_lists', 'table_style',
                  'attribution', 'xml_declaration', 'field_name_limit',
                  'option_limit', 'section_self_link', 'image_loading'):
            settings.pop(k, None)
    # Some components (math2html) print diagnostics directly to sys.stderr;
    # the command line tools show them interleaved with the system messages.
    sys.stderr = stderr
    try:
        out = core.publish_string(source, source_path=source_path,
                                  writer=writer, settings_overrides=settings)
        out = out.decode('utf-8')
        halt = False
    except SystemMessage:
        out, halt = None, True
    finally:
        sys.stderr = sys.__stderr__
    return out, stderr.getvalue(), halt


def fixtures():
    d = 'testdata/fixtures'
    n = 0
    for fn in sorted(os.listdir(d)):
        if not fn.endswith('.rst'):
            continue
        base = os.path.join(d, fn[:-4])
        with open(base + '.rst', encoding='utf-8', newline='') as f:
            source = f.read()
        overrides = {}
        if os.path.exists(base + '.json'):
            with open(base + '.json', encoding='utf-8') as f:
                overrides = json.load(f)
        errs = {}
        for ext, writer in WRITERS:
            out, err, halt = run(source, base + '.rst', writer, overrides)
            errs[ext] = err
            with open(base + '.' + ext, 'w', encoding='utf-8',
                      newline='') as f:
                f.write('*** HALT ***\n' if halt else out)
        with open(base + '.err', 'w', encoding='utf-8', newline='') as f:
            f.write(errs['pxml'])
        for ext, suffix in (('html4', '.err4'), ('html5', '.err5')):
            if errs[ext] != errs['pxml']:
                with open(base + suffix, 'w', encoding='utf-8',
                          newline='') as f:
                    f.write(errs[ext])
            elif os.path.exists(base + suffix):
                os.remove(base + suffix)
        n += 1
    return n


def tree():
    """Docutils' own documentation and functional test inputs (copied from
    the Docutils 0.23 source distribution to testdata/docutils); expected
    output is stored gzip-compressed under testdata/docutils.expected."""
    import gzip
    src = 'testdata/docutils'
    dst = 'testdata/docutils.expected'
    n = 0
    for dirpath, dirnames, filenames in sorted(os.walk(src)):
        dirnames.sort()
        for fn in sorted(filenames):
            if not fn.endswith('.rst'):
                continue
            path = os.path.join(dirpath, fn)
            rel = os.path.relpath(path, src)
            with open(path, encoding='utf-8', newline='') as f:
                source = f.read()
            os.makedirs(os.path.dirname(os.path.join(dst, rel)), exist_ok=True)
            kinds = [(ext, writer, {}) for ext, writer in WRITERS]
            pxml = run(source, path, 'pseudoxml', {})[0] or ''
            if '<math' in pxml:
                # also check the document structure independently of the
                # LaTeX -> HTML/MathML conversion
                kinds += [('html4m', 'html4', {'math_output': 'LaTeX'}),
                          ('html5m', 'html5', {'math_output': 'LaTeX'})]
            for ext, writer, overrides in kinds:
                out, err, halt = run(source, path, writer, overrides)
                data = ('*** HALT ***\n' if halt else out)
                for suffix, text in ((ext, data), (ext + '.err', err)):
                    with open(os.path.join(dst, rel + '.' + suffix + '.gz'),
                              'wb') as f:
                        with gzip.GzipFile(fileobj=f, mode='wb',
                                           mtime=0) as g:
                            g.write(text.encode('utf-8'))
            n += 1
    return n


def suites():
    d = 'testdata/suites'
    n = 0
    for fn in sorted(os.listdir(d)):
        if not fn.endswith('.json') or fn.endswith('.expected.json'):
            continue
        with open(os.path.join(d, fn), encoding='utf-8') as f:
            suite = json.load(f)
        overrides = suite.get('settings', {})
        expected = []
        for case in suite['cases']:
            entry = {'name': case['name']}
            for ext, writer in WRITERS:
                out, err, halt = run(case['input'], '<stdin>', writer,
                                     overrides)
                if halt:
                    entry[ext] = None
                else:
                    entry[ext] = out
                entry['err' if ext == 'pxml' else 'err' + ext[-1]] = err
            for k in ('err4', 'err5'):
                if entry[k] == entry['err']:
                    del entry[k]
            expected.append(entry)
            n += 1
        with open(os.path.join(d, fn[:-5] + '.expected.json'), 'w',
                  encoding='utf-8') as f:
            json.dump({'docutils': docutils.__version__, 'cases': expected},
                      f, indent=1, ensure_ascii=False)
            f.write('\n')
    return n


def record_paths():
    """Record the installation dependent directories that can appear in
    the expected output: messages about standard include files contain
    the path of Docutils' "include" directory, and a linked (not embedded)
    default stylesheet is referenced in the writer's directory.  The tests
    map these to what rstgo emits (see corpus_test.go)."""
    import docutils.writers
    from docutils.parsers.rst import states
    inc = os.path.join(os.path.dirname(states.__file__), 'include') + os.sep
    wri = os.path.dirname(docutils.writers.__file__) + os.sep
    with open('testdata/docutils_paths.txt', 'w', encoding='utf-8') as f:
        f.write('include=%s\nwriters=%s\n' % (inc, wri))


if __name__ == '__main__':
    what = sys.argv[1:] or ['fixtures', 'suites', 'tree']
    record_paths()
    if 'fixtures' in what:
        print('fixtures:', fixtures())
    if 'suites' in what:
        print('suite cases:', suites())
    if 'tree' in what:
        print('docutils tree documents:', tree())
