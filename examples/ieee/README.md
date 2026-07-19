# Executable IEEE example

This directory is both an example paper and an end-to-end test fixture. The
paper is synthetic. It is clearly marked as a smoke test and makes no academic
claim.

The format follows the [IEEE Author Center conference-template
path](https://conferences.ieeeauthorcenter.ieee.org/write-your-paper/authoring-tools-and-templates/)
and uses the [`IEEEtran` class and BibTeX style from
CTAN](https://ctan.org/pkg/ieeetran). The paper text and diagram are original
test content; they are not copied from IEEE sample prose.

It exercises the dependencies commonly found in a small IEEE-style paper:

- the `IEEEtran` conference class and BibTeX style;
- three files included with `\\input`;
- one external PNG figure selected through `\\graphicspath`;
- numbered equations, labels, cross-references, and a `booktabs` table;
- a separate BibTeX database with several citations;
- common full-image packages including `microtype`, `siunitx`, `listings`,
  `xcolor`, and `cleveref`.

The editable figure source is `figures/remote-compilation.svg`. The paper uses
the generated high-resolution `figures/remote-compilation.png`, so dependency
discovery must upload the PNG but not the SVG, this README, or unrelated files.

Run the same Docker Compose smoke test used by maintainers:

```sh
make smoke-papers
```

The script needs Docker, Docker Compose, and `curl`, but no local TeX Live. It
copies this directory to a temporary workspace, previews the exact upload
manifest, starts an isolated server, compiles with PDFLaTeX, checks the
returned artifacts and result APIs, and renders the PDF when Poppler is
available. It removes only the containers, volumes, and temporary files that
it created.

To compile the example against an already running Compose stack:

```sh
LATEXMK_PROJECT_DIR="$PWD/examples/ieee" \
  docker compose run --rm client --engine pdflatex main.tex
```

The standard IEEE class is provided by the TeX Live `ieeetran` package. Run
this example with the full server image. The sibling [`../slim`](../slim)
fixture tests the default slim image without IEEE-specific packages.
