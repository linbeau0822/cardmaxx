# CardMaxx frontend

React + TypeScript + Vite dashboard for the CardMaxx API.

```bash
npm install
npm run dev     # http://localhost:5173, proxies /api and /health to localhost:8080
npm run build   # type-check and bundle
npm run lint
```

The Go API must be running for the dashboard to show data — see the
[project README](../README.md) for the full development setup.

- `src/lib/api.ts` — typed API client; the single place that knows the backend's
  error envelope.
- `src/components/` — dashboard panels and the API status indicator.

Set `VITE_API_BASE_URL` to point a production build at a non-proxied API.
