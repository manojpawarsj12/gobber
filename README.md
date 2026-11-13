# Gobber

A fast, npm-compatible package installer for Node.js, written in Go.

Gobber reads and writes the same `package.json` and `package-lock.json` (lockfileVersion 3) as npm, so you can switch between the two on the same project. It is faster because:

- **Global store, symlinked `node_modules`.** Each `name@version` is downloaded and extracted once into a shared store. A project's `node_modules` holds only symlinks, so a reinstall is a few hundred `symlink` calls.
- **Lockfile first.** Versions already in `package-lock.json` are reused without touching the registry. A lockfile plus a warm store means zero network requests.
- **Everything in parallel.** Metadata lookups, downloads and extraction run concurrently (50 connections, HTTP/2), and each tarball starts downloading the moment its version is known.
- **Small metadata.** It requests npm's abbreviated ("corgi") metadata and fetches each package's metadata at most once per install.

## Benchmark

8 direct dependencies (express, react, react-dom, axios, lodash, typescript, vite, eslint), 181 packages in total. npm 11.20 run with `--no-audit --no-fund`. Linux (WSL2), single run each:

| Scenario | npm | gobber | Speedup |
|---|---|---|---|
| Cold: no lockfile, empty cache | 10.3 s | 1.6 s | ~6× |
| CI: lockfile + warm cache, no `node_modules` (`npm ci`) | 2.3 s | 0.035 s | ~65× |
| No-op: everything already installed | 0.57 s | 0.023 s | ~25× |

These numbers depend on your network and disk, so measure on your own project.

## Install

```sh
git clone https://github.com/manojpawarsj12/gobber.git
cd gobber
go build -o gobber ./cmd
```

Go 1.21 or newer.

## Usage

```sh
gobber install                 # install everything in package.json (alias: gobber i)
gobber i react                 # add react, saved as "^<latest>"
gobber i react@^18 lodash@4    # add with explicit ranges (kept as written)
gobber i -D typescript         # add to devDependencies (also: --save-dev; flags may come last)
gobber i --production          # skip devDependencies; the lockfile is not rewritten
gobber i sw@npm:string-width@4 # npm aliases
gobber store                   # print the store path
```

Supported version specs are semver ranges (`^1.2`, `~1`, `>=1 <2`, `1.x`, `*`), exact versions, dist-tags (`latest`, `next`) and `npm:` aliases.

### Environment variables

| Variable | Default | Meaning |
|---|---|---|
| `GOBBER_STORE` | `$XDG_CACHE_HOME/gobber/store` (`~/.cache/gobber/store`) | global package store |
| `GOBBER_REGISTRY` | `https://registry.npmjs.org` | registry URL |

## How it works

```
~/.cache/gobber/store/
  express@4.22.3/node_modules/
    express/            <- extracted tarball (real files)
    body-parser  -> ../../body-parser@1.20.3/node_modules/body-parser
    ...                 <- symlinks to exactly the versions express resolved to
project/node_modules/
  express  -> ~/.cache/gobber/store/express@4.22.3/node_modules/express
  .bin/    -> bins of direct dependencies
```

Node resolves `require()` from a module's real path. When `express` requires `body-parser`, Node looks in the store's `express@4.22.3/node_modules/` and finds the right version. Only your direct dependencies are visible from the project root, the same strictness as pnpm.

An install:

1. Reads `package.json` and `package-lock.json`.
2. Resolves every range. The highest version already in the lockfile that satisfies it wins; otherwise the registry's `latest` tag if it satisfies, otherwise the highest matching version.
3. Downloads each missing tarball, verifies its `integrity` (sha512, or sha1 for old packages), and extracts it to a temp dir that is atomically renamed into the store.
4. Links dependencies inside the store, then links the project's direct dependencies and bins.
5. Writes an npm-style hoisted `package-lock.json` (which `npm ci` accepts) and, when packages were added, `package.json`. Key order and all other fields are kept.

Optional dependencies for other platforms (`os`/`cpu`) are recorded in the lockfile but not downloaded. Peer dependencies link to the project's own copy when it satisfies the range, so there is only one React.

## Limitations

- **No lifecycle scripts** (`preinstall`/`install`/`postinstall`). Packages that build native code with node-gyp, or download binaries in `postinstall`, won't work. Most modern tools ship prebuilt binaries as optional dependencies instead, and those work.
- No git, `file:`, URL-tarball or workspace dependencies.
- Packages missing from `package.json` are not pruned from `node_modules`.
- No `.npmrc`, auth tokens or scoped registries.
- One store entry per `name@version`, so peer dependencies are not isolated per project the way pnpm does it.
- Tools that refuse to follow symlinks (rare) may misbehave, as with pnpm.

## Development

```sh
go test -race ./...
```

The tests run the full installer against a fake in-process registry. They cover the store layout, hoisting, cache hits with no network, integrity failures, platform-specific optional deps, peers, package.json editing and tar path traversal.

## License

MIT. See the LICENSE file for details.
