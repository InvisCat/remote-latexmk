# Executable slim-image example

This is the small sibling of the full [IEEE example](../ieee). It is both a
readable example and an end-to-end test fixture. The content is synthetic,
slightly circular, and clearly marked as a smoke test rather than research.

The document uses the standard `article` class and packages already expected
in the slim image. It still exercises multi-file input, an external PNG
figure, equations, cross-references, a table, and a separate BibTeX database.
It deliberately avoids `IEEEtran` and full-image-only compatibility checks.

Run both executable examples:

```sh
make smoke-papers
```

The command tests this slim article first and then the full IEEE paper. No
local TeX installation is needed.
