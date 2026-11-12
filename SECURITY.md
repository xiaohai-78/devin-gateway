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
- keep it in `.env` (gitignored) or a mounted config file — never commit it;
- if a token leaks, sign out / re-authenticate in the Devin app to rotate it.

The gateway itself stores no data and sends requests only to the endpoint you configure
(`devin.base_url`, default `https://server.codeium.com`).

## Scope

The upstream protocol is unofficial and undocumented; vulnerabilities in the upstream
service are out of scope for this project.
