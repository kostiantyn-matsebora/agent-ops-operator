#!/usr/bin/env python3
"""The `struct-table` marker in docs-generate.py: a rendered table, a drift
failure, and a loud failure on a struct the parser cannot handle.

`docs-generate.py` has a hyphen in its name, so it cannot be `import`ed by
module name the way `conveyor.py` is in conveyor.test.py -- this loads it by
path with `importlib`, the same module object `main()` itself would run,
rather than a copy. `_parse_go_struct_fields`, `_humanize_go_type` and
`build_struct_table` already take a file argument resolved against the
module's own `REPO` constant (`path = REPO / file_rel`), which is a seam:
since `pathlib`'s `/` returns an absolute right-hand side unchanged, an
ABSOLUTE fixture path would bypass `REPO` entirely for the happy path -- but
every error branch below also calls `path.relative_to(REPO)` to NAME the
file in its message, which raises `ValueError` (not `SystemExit`) for a path
outside `REPO`. So this patches the live module's `REPO` attribute to a
throwaway `tempfile.mkdtemp()` for the duration of each test, which makes
`REPO / file_rel` resolve inside the fixture tree exactly as it would resolve
inside the real repository -- the only way to exercise the real error
messages without writing anything under this checkout.

Nothing here touches the real repository: every fixture is a file written
under `tempfile.mkdtemp()` and the module's `REPO`/`DOCS` are restored after
every test.
"""
from __future__ import annotations

import importlib.util
import pathlib
import shutil
import tempfile
import textwrap
import unittest

SCRIPT = pathlib.Path(__file__).resolve().parents[1] / "scripts" / "docs-generate.py"


def _load_module():
    spec = importlib.util.spec_from_file_location("docs_generate_under_test", SCRIPT)
    mod = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(mod)
    return mod


class StructTableBase(unittest.TestCase):
    def setUp(self):
        self.m = _load_module()
        self.real_repo = self.m.REPO
        self.tmp = pathlib.Path(tempfile.mkdtemp())
        self.m.REPO = self.tmp  # the seam: every `REPO / file_rel` now resolves here

    def tearDown(self):
        self.m.REPO = self.real_repo
        shutil.rmtree(self.tmp, ignore_errors=True)

    def write_go(self, name: str, body: str) -> None:
        (self.tmp / name).write_text(textwrap.dedent(body))


class ARenderedTable(StructTableBase):
    def test_required_and_optional_fields_render_with_doc_comments_as_meaning(self):
        self.write_go("fixture.go", """\
            package fixture

            // Thing is a fixture struct for the struct-table generator test.
            type Thing struct {
            	// Name is the required field.
            	Name string `json:"name"`
            	// Note is the optional field.
            	Note *string `json:"note,omitempty"`
            }
            """)
        table = self.m.build_struct_table("fixture.go", "Thing")
        self.assertEqual(
            table,
            "| Field | Type | Meaning |\n"
            "|---|---|---|\n"
            "| `name` | string | Name is the required field. |\n"
            "| `note` | string, optional | Note is the optional field. |\n",
        )

    def test_a_trailing_comment_is_the_meaning_when_there_is_no_doc_comment(self):
        self.write_go("trailing.go", """\
            package fixture

            type Thing struct {
            	Name string `json:"name"` // the trailing comment
            }
            """)
        table = self.m.build_struct_table("trailing.go", "Thing")
        self.assertIn("| `name` | string | the trailing comment |", table)

    def test_a_json_dash_tag_is_skipped_and_resets_the_pending_doc_comment(self):
        self.write_go("skipped.go", """\
            package fixture

            type Thing struct {
            	// Internal is never rendered.
            	Internal string `json:"-"`
            	// Name is the only visible field.
            	Name string `json:"name"`
            }
            """)
        table = self.m.build_struct_table("skipped.go", "Thing")
        self.assertNotIn("Internal", table)
        self.assertIn("| `name` | string | Name is the only visible field. |", table)

    def test_a_pipe_in_the_doc_comment_is_escaped_not_a_new_column(self):
        # A doc comment written as prose enum -- `"a" | "b"` -- is a natural
        # way to describe a field, and an unescaped `|` would read as a table
        # column boundary, corrupting the row with no error anywhere.
        self.write_go("piped.go", """\
            package fixture

            type Thing struct {
            	// Status is "succeeded" | "failed".
            	Status string `json:"status"`
            }
            """)
        table = self.m.build_struct_table("piped.go", "Thing")
        self.assertIn(
            '| `status` | string | Status is "succeeded" \\| "failed". |', table
        )
        # Exactly three columns: 4 structural pipes plus the one escaped
        # `\|` inside the Meaning cell, which still counts as a `|`
        # character -- an UNESCAPED pipe there would add a fifth structural
        # one (a fourth column) instead.
        row = next(l for l in table.splitlines() if l.startswith("| `status`"))
        self.assertEqual(row.count("|"), 5)
        self.assertEqual(row.count("\\|"), 1)

    def test_an_array_field_renders_with_brackets_like_the_rest_of_the_page(self):
        # `turns[]`/`toolCalls[]` are how every OTHER mention of these
        # fields reads on the page the generated table lands in -- the
        # Field column should match, not read the bare JSON key.
        self.write_go("array.go", """\
            package fixture

            type Thing struct {
            	// Turns is one entry per model call.
            	Turns []Turn `json:"turns,omitempty"`
            }
            """)
        table = self.m.build_struct_table("array.go", "Thing")
        self.assertIn("| `turns[]` | array of Turn, optional |", table)


class ADriftFailure(StructTableBase):
    """The exact comparison `main()` uses to decide `stale`: the regenerated
    body against the body a marker currently holds."""

    def test_check_mode_rejects_a_marker_body_that_no_longer_matches_the_struct(self):
        self.write_go("fixture.go", """\
            package fixture

            type Thing struct {
            	// Name is the required field.
            	Name string `json:"name"`
            }
            """)
        doc = self.tmp / "doc.md"
        doc.write_text(
            "# Doc\n\n"
            "<!-- generated: struct-table file=fixture.go struct=Thing -->\n"
            "| Field | Type | Meaning |\n"
            "|---|---|---|\n"
            "| `name` | string | WRONG, STALE TEXT |\n"
            "<!-- /generated -->\n"
        )
        produced = self.m.rewrite(doc, {})
        # This is literally `stale = [p for p, body in produced.items() if ...
        # path.read_text() != body]` in `main()`, applied to one file.
        self.assertNotEqual(produced, doc.read_text())
        self.assertIn("Name is the required field.", produced)
        self.assertNotIn("WRONG, STALE TEXT", produced)

    def test_a_marker_whose_body_already_matches_is_not_drift(self):
        self.write_go("fixture.go", """\
            package fixture

            type Thing struct {
            	// Name is the required field.
            	Name string `json:"name"`
            }
            """)
        doc = self.tmp / "doc.md"
        doc.write_text(
            "# Doc\n\n"
            "<!-- generated: struct-table file=fixture.go struct=Thing -->\n"
            "| Field | Type | Meaning |\n"
            "|---|---|---|\n"
            "| `name` | string | Name is the required field. |\n"
            "<!-- /generated -->\n"
        )
        produced = self.m.rewrite(doc, {})
        self.assertEqual(produced, doc.read_text())


class ALoudFailureOnAnUnparsableStruct(StructTableBase):
    def test_an_unrecognized_go_type_names_the_file_struct_and_field(self):
        self.write_go("unknown_type.go", """\
            package fixture

            type Weird struct {
            	// Count is a float, a shape the generator has never been taught.
            	Count float64 `json:"count"`
            }
            """)
        with self.assertRaises(SystemExit) as ctx:
            self.m.build_struct_table("unknown_type.go", "Weird")
        msg = str(ctx.exception)
        self.assertIn("unknown_type.go", msg)
        self.assertIn("struct Weird", msg)
        self.assertIn("field Count", msg)
        self.assertIn("unrecognized Go type", msg)

    def test_a_field_with_no_doc_or_trailing_comment_is_refused(self):
        self.write_go("missing_doc.go", """\
            package fixture

            type Bare struct {
            	Name string `json:"name"`
            }
            """)
        with self.assertRaises(SystemExit) as ctx:
            self.m.build_struct_table("missing_doc.go", "Bare")
        msg = str(ctx.exception)
        self.assertIn("missing_doc.go", msg)
        self.assertIn("field Name", msg)
        self.assertIn("no doc comment", msg)

    def test_a_struct_name_absent_from_the_file_is_refused(self):
        self.write_go("no_such_struct.go", """\
            package fixture

            type Other struct {
            	Name string `json:"name"`
            }
            """)
        with self.assertRaises(SystemExit) as ctx:
            self.m.build_struct_table("no_such_struct.go", "DoesNotExist")
        msg = str(ctx.exception)
        self.assertIn("no_such_struct.go", msg)
        self.assertIn("DoesNotExist", msg)

    def test_a_field_line_that_does_not_match_the_flat_shape_is_refused(self):
        self.write_go("unparsable.go", """\
            package fixture

            type Odd struct {
            	Name string no backticks at all
            }
            """)
        with self.assertRaises(SystemExit) as ctx:
            self.m.build_struct_table("unparsable.go", "Odd")
        msg = str(ctx.exception)
        self.assertIn("unparsable.go", msg)
        self.assertIn("struct Odd", msg)
        self.assertIn("cannot parse field line", msg)

    def test_no_such_file_is_refused_by_name_not_a_traceback(self):
        with self.assertRaises(SystemExit) as ctx:
            self.m.build_struct_table("never-written.go", "Anything")
        self.assertIn("never-written.go", str(ctx.exception))

    def test_the_failure_surfaces_through_rewrite_naming_the_doc_and_the_marker(self):
        """The path `main()` actually takes: a marker in a page, not a direct call."""
        self.write_go("unknown_type.go", """\
            package fixture

            type Weird struct {
            	// Count is a float.
            	Count float64 `json:"count"`
            }
            """)
        doc = self.tmp / "doc.md"
        doc.write_text(
            "# Doc\n\n"
            "<!-- generated: struct-table file=unknown_type.go struct=Weird -->\n"
            "placeholder\n"
            "<!-- /generated -->\n"
        )
        with self.assertRaises(SystemExit) as ctx:
            self.m.rewrite(doc, {})
        msg = str(ctx.exception)
        self.assertIn("doc.md", msg)
        self.assertIn("in marker:", msg)
        self.assertIn("struct-table file=unknown_type.go struct=Weird", msg)


if __name__ == "__main__":
    unittest.main()
