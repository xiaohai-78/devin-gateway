# Security Policy

## Supported versions

Only the latest release (or `main`) receives security fixes.

## Reporting a vulnerability

Please **do not** open a public issue for a security problem.
Use GitHub's private vulnerability reporting on this repository
("Security" → "Report a vulnerability"), or contact the maintainer listed on their GitHub profile.

## Secrets

This gateway authenticates to the upstream with a Devin **session token**, which is
equivalent to an account credential:

- never paste a token into an issue, discussion, pull request, or log;
- keep it in `.env` (gitignored) or your own `config.yaml` (also gitignored) — never commit it;
- don't store it in persistent user/machine environment variables (e.g. Windows `setx`): every
  program you run could read it. `scripts/start-windows.ps1` loads `.env` for the gateway process only;
- if a token leaks, sign out / re-authenticate in the Devin app to rotate it.

The gateway itself stores no data and sends requests only to the endpoint you configure
(`devin.base_url`, default `https://server.codeium.com`).

## Network exposure

The gateway uses your account quota, and authentication is off by default. It therefore listens on
`127.0.0.1` only (Docker: the container port is published on `127.0.0.1`). Before exposing it to
any other machine, set `AUTH_API_KEY`; prefer Tailscale or an SSH tunnel over opening the port.

## Scope

The upstream protocol is unofficial and undocumented; vulnerabilities in the upstream
service are out of scope for this project.
