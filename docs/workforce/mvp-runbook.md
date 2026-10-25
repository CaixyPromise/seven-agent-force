# Local operations

Read [deployment](../deployment.md) and [configuration](../configuration.md) first. Supply your own database, queue, object storage and authentication configuration using the documented examples. Never commit real configuration.

Build and check the API and console with `make check` and `make build`. Start the API using its supplied command/configuration options. Provision a Worker through the operator console or authenticated `POST /api/workforce/workers`, save its token privately, then let `seven-worker` register its process session. `seven-workerctl` provides health/task/artifact operations; it does not provision Workers. See [containerized Worker setup](worker-container.md) and the [Worker protocol reference](protocols/README.md). Publish a compatible Runtime with an immutable image digest before scheduling tasks.

Use a synthetic task in an isolated tenant to verify scheduling, review, result download and authorized Git delivery. A successful code check does not establish business acceptance. Review cancellation, input authorization and cold recovery in the target installation.
