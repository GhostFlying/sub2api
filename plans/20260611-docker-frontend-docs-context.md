## Plan

- Keep Docker documentation exclusions broad, but allow the two admin
  compliance markdown files needed by frontend raw imports.
- Copy those files into `/app/docs/legal/` in the frontend builder stage so the
  existing relative imports resolve during `pnpm run build`.
- Validate with frontend build checks and workflow lint before pushing the
  follow-up commit.
