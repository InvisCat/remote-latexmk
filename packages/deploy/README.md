# @latexmk/deploy

TypeScript deployment bundler for the latexmk server. It produces a standalone
Docker build context for either the XeLaTeX/CJK or full TeX Live profile,
`.env.example`, `compose.yaml`, and deployment metadata.

```sh
pnpm --filter @latexmk/deploy build
node packages/deploy/dist/index.js bundle --profile slim --auth token --out dist/slim
```

For an existing PostgreSQL service and a low-cost deployment profile:

```sh
node packages/deploy/dist/index.js bundle \
  --profile slim \
  --preset railway \
  --auth postgres --database postgres --external-database \
  --out dist/railway
```

Available resource presets are `railway-serverless`, `lightsail-tokyo`, and
`railway`. They set bounded compiler, queue, memory, tmpfs, state-cache, and
retention policies. See `docs/OPERATIONS.md` for their values and trade-offs.

`--database pglite` generates a PGlite socket service for local development or
demonstration only. It does not provide full PostgreSQL's concurrency or TLS.
