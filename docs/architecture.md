# Architecture

## System boundary

Seven Framework is distributed as one Go API service plus Web assets. Nginx serves the SPA and forwards only `/api/` to the Go service. The Web build is not an npm package and the Go service does not embed or serve the SPA in the first public release.

## Go domain architecture

Application modules live under `seven-agent-force-server/internal/app`. A module may expose these layers:

- `controller`: HTTP entry points;
- `job`: scheduled entry points;
- `listener`: message-consumer entry points;
- `facade`: stable cross-module and external application contracts;
- `application`: use-case orchestration and transaction boundaries;
- `domain`: aggregates, value objects, policies, and core business rules;
- `infrastructure`: persistence and provider adapters.

Allowed directions are entry point to local facade/application, application to domain/facade, facade to domain, and domain to infrastructure ports. Infrastructure cannot import higher-level application packages. Cross-module use is through API or facade contracts rather than another module's internal application implementation.

The test suite contains import-boundary checks. A directory rename or provider integration must keep those checks green.

## Shared infrastructure

The bootstrap layer assembles modules and adapters. Shared infrastructure provides database access, caching, RabbitMQ, observability, security primitives, and HTTP protocol support. Optional infrastructure is activated by configuration; disabled features must not silently become partially active.

## Web application

`seven-agent-force-web` is a React, TypeScript, Vite, Ant Design, and Pro Components application. It consumes same-origin `/api` routes in production. Runtime branding and feature availability come from API contracts rather than hard-coded private deployment values.
