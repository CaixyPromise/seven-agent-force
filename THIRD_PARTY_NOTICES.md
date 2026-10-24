# Third-party software notices

Seven Agent Force source is Apache-2.0. Third-party software retains its own
licenses, attribution and conditions; the project license does not replace them.
This inventory covers locked active Web dependencies (including build/optional
packages), active Go requirements, and dependencies observed in the six release
binaries. Runtime Docker images and their separately installed third-party Coding
CLIs are outside this archive and require their own notices before distribution.

Full retained license/attribution evidence is in `third_party/licenses/`.
`third_party/licenses.json` records exact versions, source URLs, scope and SHA256
for each evidence file. File names are content hashes to avoid collisions.

## Specific obligations and scope decisions

- `github.com/go-sql-driver/mysql v1.9.3` uses MPL-2.0. The release includes its
  unchanged corresponding source archive under `third_party/sources/`; the
  exact upstream URL and checksum are also in the inventory. Retain its MPL
  notices. Any future changes to covered files must be made available under MPL.
  The larger application remains under its own license. See
  https://www.mozilla.org/en-US/MPL/2.0/FAQ/ and the bundled MPL text.
- Lightning CSS uses MPL-2.0 as a build tool. Its Node binaries/source are not
  shipped in this API/Web archive. Its versions and notices are retained in the
  conservative build inventory; shipping it separately requires corresponding
  source and MPL notices as well.
- `github.com/golang/freetype` offers FTL or GPL. This distribution selects FTL,
  preserves `LICENSE` and `licenses/ftl.txt`, retains
  Copyright 2010 The Freetype-Go Authors. All rights reserved., and acknowledges:
  Portions of this software are copyright 1996-2002, 2006 The FreeType Project
  (www.freetype.org). All rights reserved. Separately licensed test fonts under
  its testdata directory are not included in the binary or archive.
- The GPL notice detected inside `modernc.org/libc/testdata/.../crlibm` applies to
  upstream test data, not the module's BSD-3-Clause runtime. That test data is
  not part of the CGO-disabled binary or release archive. Its root license is
  retained. No GPL exception is granted to compiled code by this determination.
- MCP Go SDK's transition notice includes Apache-2.0, remaining MIT contributions
  and CC-BY-4.0 documentation. Preserve its full notice; do not rewrite the whole
  module's history as exclusively Apache. OpenContainers documentation has a
  separate CC-BY-SA license and is not included in the runtime package.
- Four build/other-platform packages publish a license declaration but no
  standalone full text at the resolved version: keyv, natural-compare,
  @tybys/wasm-util and @tailwindcss/oxide-wasm32-wasi. Their public exact-version
  metadata is retained as declaration-only evidence. Their code is not shipped.
  The release gate rejects using declaration-only evidence for runtime packages.
- The formerly used @ant-design/use-emotion-css wrapper was removed from active
  Web dependencies because its license text/attribution could not be recovered.
  The authentication pages call the already-used MIT-licensed Emotion CSS API
  directly. Pinned Framework reference source is unchanged and not built into
  this release; installing or redistributing that reference separately requires
  its own dependency review.

## Dependency inventory

The following table is generated from reviewed version-specific evidence.
`build-or-optional-dependency` is a conservative source/build inventory, not a
claim that those packages' executable code is bundled in Web assets.

| Ecosystem | Package | Version | License | Scope |
| --- | --- | --- | --- | --- |
| go | filippo.io/edwards25519 | v1.2.0 | BSD-3-Clause | binary-or-module-graph |
| go | github.com/DATA-DOG/go-sqlmock | v1.5.2 | BSD-3-Clause | binary-or-module-graph |
| go | github.com/Microsoft/go-winio | v0.6.2 | MIT | binary-or-module-graph |
| go | github.com/alicebob/miniredis/v2 | v2.37.0 | BSL-1.0 AND MIT AND Unlicense | binary-or-module-graph |
| go | github.com/beorn7/perks | v1.0.1 | MIT | binary-or-module-graph |
| go | github.com/boombuler/barcode | v1.0.1-0.20190219062509-6c824513bacc | MIT | binary-or-module-graph |
| go | github.com/bwmarrin/snowflake | v0.3.0 | BSD-2-Clause | binary-or-module-graph |
| go | github.com/bytedance/gopkg | v0.1.3 | Apache-2.0 | binary-or-module-graph |
| go | github.com/bytedance/sonic | v1.15.0 | Apache-2.0 | binary-or-module-graph |
| go | github.com/bytedance/sonic/loader | v0.5.0 | Apache-2.0 | binary-or-module-graph |
| go | github.com/cenkalti/backoff/v5 | v5.0.3 | MIT | binary-or-module-graph |
| go | github.com/cespare/xxhash/v2 | v2.3.0 | MIT | binary-or-module-graph |
| go | github.com/cloudwego/base64x | v0.1.6 | Apache-2.0 | binary-or-module-graph |
| go | github.com/cloudwego/gopkg | v0.1.4 | Apache-2.0 | binary-or-module-graph |
| go | github.com/cloudwego/hertz | v0.10.4 | Apache-2.0 | binary-or-module-graph |
| go | github.com/cloudwego/netpoll | v0.7.2 | Apache-2.0 | binary-or-module-graph |
| go | github.com/coder/websocket | v1.8.14 | ISC | binary-or-module-graph |
| go | github.com/containerd/errdefs | v1.0.0 | Apache-2.0 | binary-or-module-graph |
| go | github.com/containerd/errdefs/pkg | v0.3.0 | Apache-2.0 | binary-or-module-graph |
| go | github.com/containerd/log | v0.1.0 | Apache-2.0 | binary-or-module-graph |
| go | github.com/decred/dcrd/dcrec/secp256k1/v4 | v4.4.1 | ISC | binary-or-module-graph |
| go | github.com/dgraph-io/ristretto | v0.2.0 | Apache-2.0 AND MIT | binary-or-module-graph |
| go | github.com/dgryski/go-rendezvous | v0.0.0-20200823014737-9f7001d12a5f | MIT | binary-or-module-graph |
| go | github.com/distribution/reference | v0.6.0 | Apache-2.0 | binary-or-module-graph |
| go | github.com/docker/docker | v28.5.2+incompatible | Apache-2.0 AND MIT | binary-or-module-graph |
| go | github.com/docker/go-connections | v0.7.0 | Apache-2.0 | binary-or-module-graph |
| go | github.com/docker/go-units | v0.5.0 | Apache-2.0 | binary-or-module-graph |
| go | github.com/dustin/go-humanize | v1.0.1 | MIT | binary-or-module-graph |
| go | github.com/ebitengine/purego | v0.10.2 | Apache-2.0 | binary-or-module-graph |
| go | github.com/felixge/fgprof | v0.9.3 | MIT | binary-or-module-graph |
| go | github.com/felixge/httpsnoop | v1.0.4 | MIT | binary-or-module-graph |
| go | github.com/fsnotify/fsnotify | v1.9.0 | BSD-3-Clause | binary-or-module-graph |
| go | github.com/gabriel-vasile/mimetype | v1.4.10 | MIT | binary-or-module-graph |
| go | github.com/go-ini/ini | v1.67.0 | Apache-2.0 | binary-or-module-graph |
| go | github.com/go-logr/logr | v1.4.4 | Apache-2.0 | binary-or-module-graph |
| go | github.com/go-logr/stdr | v1.2.2 | Apache-2.0 | binary-or-module-graph |
| go | github.com/go-ole/go-ole | v1.2.6 | MIT | binary-or-module-graph |
| go | github.com/go-playground/locales | v0.14.1 | MIT | binary-or-module-graph |
| go | github.com/go-playground/universal-translator | v0.18.1 | MIT | binary-or-module-graph |
| go | github.com/go-playground/validator/v10 | v10.28.0 | MIT | binary-or-module-graph |
| go | github.com/go-sql-driver/mysql | v1.9.3 | MPL-2.0 | binary-or-module-graph |
| go | github.com/go-viper/mapstructure/v2 | v2.4.0 | MIT | binary-or-module-graph |
| go | github.com/goccy/go-json | v0.10.6 | MIT | binary-or-module-graph |
| go | github.com/golang/freetype | v0.0.0-20170609003504-e2365dfdc4a0 | FTL | binary-or-module-graph |
| go | github.com/google/jsonschema-go | v0.4.3 | MIT | binary-or-module-graph |
| go | github.com/google/pprof | v0.0.0-20250317173921-a4b03ec1a45e | Apache-2.0 AND BSD-3-Clause | binary-or-module-graph |
| go | github.com/google/uuid | v1.6.0 | BSD-3-Clause | binary-or-module-graph |
| go | github.com/grpc-ecosystem/grpc-gateway/v2 | v2.29.0 | BSD-3-Clause | binary-or-module-graph |
| go | github.com/hertz-contrib/pprof | v0.1.2 | Apache-2.0 | binary-or-module-graph |
| go | github.com/jackc/pgpassfile | v1.0.0 | MIT | binary-or-module-graph |
| go | github.com/jackc/pgservicefile | v0.0.0-20240606120523-5a60cdf6a761 | MIT | binary-or-module-graph |
| go | github.com/jackc/pgx/v5 | v5.9.2 | MIT | binary-or-module-graph |
| go | github.com/jackc/puddle/v2 | v2.2.2 | MIT | binary-or-module-graph |
| go | github.com/jmoiron/sqlx | v1.4.0 | MIT | binary-or-module-graph |
| go | github.com/klauspost/compress | v1.18.4 | Apache-2.0 AND BSD-3-Clause AND MIT | binary-or-module-graph |
| go | github.com/klauspost/cpuid/v2 | v2.2.11 | MIT | binary-or-module-graph |
| go | github.com/klauspost/crc32 | v1.3.0 | BSD-3-Clause | binary-or-module-graph |
| go | github.com/leodido/go-urn | v1.4.0 | MIT | binary-or-module-graph |
| go | github.com/lestrrat-go/blackmagic | v1.0.4 | MIT | binary-or-module-graph |
| go | github.com/lestrrat-go/dsig | v1.2.1 | MIT | binary-or-module-graph |
| go | github.com/lestrrat-go/dsig-secp256k1 | v1.0.0 | MIT | binary-or-module-graph |
| go | github.com/lestrrat-go/httpcc | v1.0.1 | MIT | binary-or-module-graph |
| go | github.com/lestrrat-go/httprc/v3 | v3.0.5 | MIT | binary-or-module-graph |
| go | github.com/lestrrat-go/jwx/v3 | v3.1.0 | MIT | binary-or-module-graph |
| go | github.com/lestrrat-go/option/v2 | v2.0.0 | MIT | binary-or-module-graph |
| go | github.com/lufia/plan9stats | v0.0.0-20211012122336-39d0f177ccd0 | BSD-3-Clause | binary-or-module-graph |
| go | github.com/mattn/go-isatty | v0.0.20 | MIT | binary-or-module-graph |
| go | github.com/mfridman/interpolate | v0.0.2 | MIT | binary-or-module-graph |
| go | github.com/minio/crc64nvme | v1.1.1 | Apache-2.0 | binary-or-module-graph |
| go | github.com/minio/md5-simd | v1.1.2 | Apache-2.0 | binary-or-module-graph |
| go | github.com/minio/minio-go/v7 | v7.0.100 | Apache-2.0 | binary-or-module-graph |
| go | github.com/moby/docker-image-spec | v1.3.1 | Apache-2.0 | binary-or-module-graph |
| go | github.com/moby/sys/atomicwriter | v0.1.0 | Apache-2.0 | binary-or-module-graph |
| go | github.com/moby/term | v0.5.2 | Apache-2.0 | binary-or-module-graph |
| go | github.com/modelcontextprotocol/go-sdk | v1.7.0 | Apache-2.0 AND CC-BY-4.0 AND MIT | binary-or-module-graph |
| go | github.com/mojocn/base64Captcha | v1.3.8 | Apache-2.0 | binary-or-module-graph |
| go | github.com/morikuni/aec | v1.1.0 | MIT | binary-or-module-graph |
| go | github.com/munnerz/goautoneg | v0.0.0-20191010083416-a7dc8b61c822 | BSD-3-Clause | binary-or-module-graph |
| go | github.com/ncruces/go-strftime | v1.0.0 | MIT | binary-or-module-graph |
| go | github.com/opencontainers/go-digest | v1.0.0 | Apache-2.0 | binary-or-module-graph |
| go | github.com/opencontainers/image-spec | v1.1.1 | Apache-2.0 | binary-or-module-graph |
| go | github.com/pelletier/go-toml/v2 | v2.2.4 | MIT | binary-or-module-graph |
| go | github.com/philhofer/fwd | v1.2.0 | MIT | binary-or-module-graph |
| go | github.com/pkg/errors | v0.9.1 | BSD-2-Clause | binary-or-module-graph |
| go | github.com/power-devops/perfstat | v0.0.0-20240221224432-82ca36839d55 | MIT | binary-or-module-graph |
| go | github.com/pquerna/otp | v1.5.0 | Apache-2.0 | binary-or-module-graph |
| go | github.com/pressly/goose/v3 | v3.27.0 | MIT | binary-or-module-graph |
| go | github.com/prometheus/client_golang | v1.23.2 | Apache-2.0 AND BSD-3-Clause | binary-or-module-graph |
| go | github.com/prometheus/client_model | v0.6.2 | Apache-2.0 | binary-or-module-graph |
| go | github.com/prometheus/common | v0.67.5 | Apache-2.0 | binary-or-module-graph |
| go | github.com/prometheus/otlptranslator | v1.0.0 | Apache-2.0 | binary-or-module-graph |
| go | github.com/prometheus/procfs | v0.20.1 | Apache-2.0 | binary-or-module-graph |
| go | github.com/rabbitmq/amqp091-go | v1.14.0 | BSD-2-Clause | binary-or-module-graph |
| go | github.com/redis/go-redis/v9 | v9.17.0 | BSD-2-Clause | binary-or-module-graph |
| go | github.com/remyoudompheng/bigfft | v0.0.0-20230129092748-24d4a6f8daec | BSD-3-Clause | binary-or-module-graph |
| go | github.com/richardlehane/mscfb | v1.0.8 | Apache-2.0 | binary-or-module-graph |
| go | github.com/richardlehane/msoleps | v1.0.6 | Apache-2.0 | binary-or-module-graph |
| go | github.com/robfig/cron/v3 | v3.0.1 | MIT | binary-or-module-graph |
| go | github.com/rs/xid | v1.6.0 | MIT | binary-or-module-graph |
| go | github.com/sagikazarmark/locafero | v0.11.0 | MIT | binary-or-module-graph |
| go | github.com/segmentio/asm | v1.2.1 | MIT-0 | binary-or-module-graph |
| go | github.com/segmentio/encoding | v0.5.4 | Apache-2.0 AND MIT | binary-or-module-graph |
| go | github.com/sethvargo/go-retry | v0.3.0 | Apache-2.0 | binary-or-module-graph |
| go | github.com/shirou/gopsutil/v4 | v4.26.7 | BSD-3-Clause | binary-or-module-graph |
| go | github.com/sourcegraph/conc | v0.3.1-0.20240121214520-5f936abd7ae8 | MIT | binary-or-module-graph |
| go | github.com/spf13/afero | v1.15.0 | Apache-2.0 | binary-or-module-graph |
| go | github.com/spf13/cast | v1.10.0 | MIT | binary-or-module-graph |
| go | github.com/spf13/pflag | v1.0.10 | BSD-3-Clause | binary-or-module-graph |
| go | github.com/spf13/viper | v1.21.0 | MIT | binary-or-module-graph |
| go | github.com/subosito/gotenv | v1.6.0 | MIT | binary-or-module-graph |
| go | github.com/tidwall/gjson | v1.14.4 | MIT | binary-or-module-graph |
| go | github.com/tidwall/match | v1.1.1 | MIT | binary-or-module-graph |
| go | github.com/tidwall/pretty | v1.2.0 | MIT | binary-or-module-graph |
| go | github.com/tiendc/go-deepcopy | v1.7.2 | MIT | binary-or-module-graph |
| go | github.com/tinylib/msgp | v1.6.1 | MIT | binary-or-module-graph |
| go | github.com/tklauser/go-sysconf | v0.3.16 | BSD-3-Clause | binary-or-module-graph |
| go | github.com/tklauser/numcpus | v0.11.0 | Apache-2.0 | binary-or-module-graph |
| go | github.com/twitchyliquid64/golang-asm | v0.15.1 | BSD-3-Clause | binary-or-module-graph |
| go | github.com/valyala/fastjson | v1.6.10 | MIT | binary-or-module-graph |
| go | github.com/xuri/efp | v0.0.1 | BSD-3-Clause | binary-or-module-graph |
| go | github.com/xuri/excelize/v2 | v2.11.1-0.20260915055537-22f76f9acb94 | BSD-3-Clause | binary-or-module-graph |
| go | github.com/xuri/nfp | v0.0.2-0.20250530014748-2ddeb826f9a9 | BSD-3-Clause | binary-or-module-graph |
| go | github.com/yosida95/uritemplate/v3 | v3.0.2 | BSD-3-Clause | binary-or-module-graph |
| go | github.com/yuin/gopher-lua | v1.1.1 | MIT | binary-or-module-graph |
| go | github.com/yusufpapurcu/wmi | v1.2.4 | MIT | binary-or-module-graph |
| go | go.opentelemetry.io/auto/sdk | v1.2.1 | Apache-2.0 | binary-or-module-graph |
| go | go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp | v0.65.0 | Apache-2.0 AND BSD-3-Clause | binary-or-module-graph |
| go | go.opentelemetry.io/otel | v1.45.0 | Apache-2.0 AND BSD-3-Clause | binary-or-module-graph |
| go | go.opentelemetry.io/otel/exporters/otlp/otlptrace | v1.45.0 | Apache-2.0 AND BSD-3-Clause | binary-or-module-graph |
| go | go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp | v1.45.0 | Apache-2.0 AND BSD-3-Clause | binary-or-module-graph |
| go | go.opentelemetry.io/otel/exporters/prometheus | v0.65.0 | Apache-2.0 AND BSD-3-Clause | binary-or-module-graph |
| go | go.opentelemetry.io/otel/metric | v1.45.0 | Apache-2.0 AND BSD-3-Clause | binary-or-module-graph |
| go | go.opentelemetry.io/otel/sdk | v1.45.0 | Apache-2.0 AND BSD-3-Clause | binary-or-module-graph |
| go | go.opentelemetry.io/otel/sdk/metric | v1.45.0 | Apache-2.0 AND BSD-3-Clause | binary-or-module-graph |
| go | go.opentelemetry.io/otel/trace | v1.45.0 | Apache-2.0 AND BSD-3-Clause | binary-or-module-graph |
| go | go.opentelemetry.io/proto/otlp | v1.11.0 | Apache-2.0 | binary-or-module-graph |
| go | go.uber.org/multierr | v1.11.0 | MIT | binary-or-module-graph |
| go | go.uber.org/zap | v1.27.1 | MIT | binary-or-module-graph |
| go | go.yaml.in/yaml/v2 | v2.4.4 | Apache-2.0 | binary-or-module-graph |
| go | go.yaml.in/yaml/v3 | v3.0.4 | Apache-2.0 AND MIT | binary-or-module-graph |
| go | golang.org/x/arch | v0.0.0-20210923205945-b76863e36670 | BSD-3-Clause | binary-or-module-graph |
| go | golang.org/x/crypto | v0.57.0 | BSD-3-Clause | binary-or-module-graph |
| go | golang.org/x/exp | v0.0.0-20260218203240-3dfff04db8fa | BSD-3-Clause | binary-or-module-graph |
| go | golang.org/x/image | v0.45.0 | BSD-3-Clause | binary-or-module-graph |
| go | golang.org/x/net | v0.60.0 | BSD-3-Clause | binary-or-module-graph |
| go | golang.org/x/oauth2 | v0.36.0 | BSD-3-Clause | binary-or-module-graph |
| go | golang.org/x/sync | v0.23.0 | BSD-3-Clause | binary-or-module-graph |
| go | golang.org/x/sys | v0.48.0 | BSD-3-Clause | binary-or-module-graph |
| go | golang.org/x/text | v0.42.0 | BSD-3-Clause | binary-or-module-graph |
| go | golang.org/x/time | v0.15.0 | BSD-3-Clause | binary-or-module-graph |
| go | google.golang.org/genproto/googleapis/api | v0.0.0-20260803160001-6ac0973c030d | Apache-2.0 | binary-or-module-graph |
| go | google.golang.org/genproto/googleapis/rpc | v0.0.0-20260803160001-6ac0973c030d | Apache-2.0 | binary-or-module-graph |
| go | google.golang.org/grpc | v1.83.1 | Apache-2.0 | binary-or-module-graph |
| go | google.golang.org/protobuf | v1.36.11 | BSD-3-Clause | binary-or-module-graph |
| go | gopkg.in/natefinch/lumberjack.v2 | v2.2.1 | MIT | binary-or-module-graph |
| go | gotest.tools/v3 | v3.5.2 | Apache-2.0 AND BSD-3-Clause | binary-or-module-graph |
| go | modernc.org/libc | v1.68.0 | BSD-3-Clause | binary-or-module-graph |
| go | modernc.org/mathutil | v1.7.1 | BSD-3-Clause | binary-or-module-graph |
| go | modernc.org/memory | v1.11.0 | BSD-3-Clause | binary-or-module-graph |
| go | modernc.org/sqlite | v1.46.1 | BSD-3-Clause | binary-or-module-graph |
| go | stdlib | go1.26.10 | BSD-3-Clause | binary-runtime |
| npm | @alloc/quick-lru | 5.2.0 | MIT | build-or-optional-dependency |
| npm | @ant-design/colors | 8.0.1 | MIT | web-production-dependency |
| npm | @ant-design/cssinjs | 2.1.2 | MIT | web-production-dependency |
| npm | @ant-design/cssinjs-utils | 2.1.2 | MIT | web-production-dependency |
| npm | @ant-design/fast-color | 3.0.1 | MIT | web-production-dependency |
| npm | @ant-design/icons | 6.1.0 | MIT | web-production-dependency |
| npm | @ant-design/icons-svg | 4.4.2 | MIT | web-production-dependency |
| npm | @ant-design/pro-components | 3.1.12-0 | MIT | web-production-dependency |
| npm | @ant-design/react-slick | 2.0.0 | MIT | web-production-dependency |
| npm | @babel/code-frame | 7.29.0 | MIT | web-production-dependency |
| npm | @babel/compat-data | 7.29.0 | MIT | build-or-optional-dependency |
| npm | @babel/core | 7.29.0 | MIT | build-or-optional-dependency |
| npm | @babel/generator | 7.29.1 | MIT | web-production-dependency |
| npm | @babel/helper-compilation-targets | 7.28.6 | MIT | build-or-optional-dependency |
| npm | @babel/helper-globals | 7.28.0 | MIT | web-production-dependency |
| npm | @babel/helper-module-imports | 7.28.6 | MIT | web-production-dependency |
| npm | @babel/helper-module-transforms | 7.28.6 | MIT | build-or-optional-dependency |
| npm | @babel/helper-string-parser | 7.27.1 | MIT | web-production-dependency |
| npm | @babel/helper-validator-identifier | 7.28.5 | MIT | web-production-dependency |
| npm | @babel/helper-validator-option | 7.27.1 | MIT | build-or-optional-dependency |
| npm | @babel/helpers | 7.29.2 | MIT | build-or-optional-dependency |
| npm | @babel/parser | 7.29.2 | MIT | web-production-dependency |
| npm | @babel/runtime | 7.29.2 | MIT | web-production-dependency |
| npm | @babel/template | 7.28.6 | MIT | web-production-dependency |
| npm | @babel/traverse | 7.29.0 | MIT | web-production-dependency |
| npm | @babel/types | 7.29.0 | MIT | web-production-dependency |
| npm | @ctrl/tinycolor | 4.2.0 | MIT | web-production-dependency |
| npm | @dnd-kit/accessibility | 3.1.1 | MIT | web-production-dependency |
| npm | @dnd-kit/core | 6.3.1 | MIT | web-production-dependency |
| npm | @dnd-kit/modifiers | 9.0.0 | MIT | web-production-dependency |
| npm | @dnd-kit/sortable | 10.0.0 | MIT | web-production-dependency |
| npm | @dnd-kit/utilities | 3.2.2 | MIT | web-production-dependency |
| npm | @emnapi/core | 1.9.0 | MIT | build-or-optional-dependency |
| npm | @emnapi/runtime | 1.9.0 | MIT | build-or-optional-dependency |
| npm | @emnapi/wasi-threads | 1.2.0 | MIT | build-or-optional-dependency |
| npm | @emotion/babel-plugin | 11.13.5 | MIT | web-production-dependency |
| npm | @emotion/cache | 11.14.0 | MIT | web-production-dependency |
| npm | @emotion/css | 11.13.5 | MIT | web-production-dependency |
| npm | @emotion/hash | 0.8.0 | MIT | web-production-dependency |
| npm | @emotion/hash | 0.9.2 | MIT | web-production-dependency |
| npm | @emotion/memoize | 0.9.0 | MIT | web-production-dependency |
| npm | @emotion/serialize | 1.3.3 | MIT | web-production-dependency |
| npm | @emotion/sheet | 1.4.0 | MIT | web-production-dependency |
| npm | @emotion/unitless | 0.10.0 | MIT | web-production-dependency |
| npm | @emotion/unitless | 0.7.5 | MIT | web-production-dependency |
| npm | @emotion/utils | 1.4.2 | MIT | web-production-dependency |
| npm | @emotion/weak-memoize | 0.4.0 | MIT | web-production-dependency |
| npm | @eslint-community/eslint-utils | 4.9.1 | MIT | build-or-optional-dependency |
| npm | @eslint-community/regexpp | 4.12.2 | MIT | build-or-optional-dependency |
| npm | @eslint/config-array | 0.21.2 | Apache-2.0 | build-or-optional-dependency |
| npm | @eslint/config-helpers | 0.4.2 | Apache-2.0 | build-or-optional-dependency |
| npm | @eslint/core | 0.17.0 | Apache-2.0 | build-or-optional-dependency |
| npm | @eslint/eslintrc | 3.3.5 | MIT | build-or-optional-dependency |
| npm | @eslint/js | 9.39.4 | MIT | build-or-optional-dependency |
| npm | @eslint/object-schema | 2.1.7 | Apache-2.0 | build-or-optional-dependency |
| npm | @eslint/plugin-kit | 0.4.1 | Apache-2.0 | build-or-optional-dependency |
| npm | @humanfs/core | 0.19.1 | Apache-2.0 | build-or-optional-dependency |
| npm | @humanfs/node | 0.16.7 | Apache-2.0 | build-or-optional-dependency |
| npm | @humanwhocodes/module-importer | 1.0.1 | Apache-2.0 | build-or-optional-dependency |
| npm | @humanwhocodes/retry | 0.4.3 | Apache-2.0 | build-or-optional-dependency |
| npm | @jridgewell/gen-mapping | 0.3.13 | MIT | web-production-dependency |
| npm | @jridgewell/remapping | 2.3.5 | MIT | build-or-optional-dependency |
| npm | @jridgewell/resolve-uri | 3.1.2 | MIT | web-production-dependency |
| npm | @jridgewell/sourcemap-codec | 1.5.5 | MIT | web-production-dependency |
| npm | @jridgewell/trace-mapping | 0.3.31 | MIT | web-production-dependency |
| npm | @napi-rs/wasm-runtime | 1.1.1 | MIT | build-or-optional-dependency |
| npm | @oxc-project/types | 0.120.0 | MIT | build-or-optional-dependency |
| npm | @rc-component/async-validator | 5.1.0 | MIT | web-production-dependency |
| npm | @rc-component/cascader | 1.14.0 | MIT | web-production-dependency |
| npm | @rc-component/checkbox | 2.0.0 | MIT | web-production-dependency |
| npm | @rc-component/collapse | 1.2.0 | MIT | web-production-dependency |
| npm | @rc-component/color-picker | 3.1.1 | MIT | web-production-dependency |
| npm | @rc-component/context | 2.0.1 | MIT | web-production-dependency |
| npm | @rc-component/dialog | 1.8.4 | MIT | web-production-dependency |
| npm | @rc-component/drawer | 1.4.2 | MIT | web-production-dependency |
| npm | @rc-component/dropdown | 1.0.2 | MIT | web-production-dependency |
| npm | @rc-component/form | 1.7.2 | MIT | web-production-dependency |
| npm | @rc-component/image | 1.6.0 | MIT | web-production-dependency |
| npm | @rc-component/input | 1.1.2 | MIT | web-production-dependency |
| npm | @rc-component/input-number | 1.6.2 | MIT | web-production-dependency |
| npm | @rc-component/mentions | 1.6.0 | MIT | web-production-dependency |
| npm | @rc-component/menu | 1.2.0 | MIT | web-production-dependency |
| npm | @rc-component/mini-decimal | 1.1.3 | MIT | web-production-dependency |
| npm | @rc-component/motion | 1.3.1 | MIT | web-production-dependency |
| npm | @rc-component/mutate-observer | 2.0.1 | MIT | web-production-dependency |
| npm | @rc-component/notification | 1.2.0 | MIT | web-production-dependency |
| npm | @rc-component/overflow | 1.0.0 | MIT | web-production-dependency |
| npm | @rc-component/pagination | 1.2.0 | MIT | web-production-dependency |
| npm | @rc-component/picker | 1.9.1 | MIT | web-production-dependency |
| npm | @rc-component/portal | 2.2.0 | MIT | web-production-dependency |
| npm | @rc-component/progress | 1.0.2 | MIT | web-production-dependency |
| npm | @rc-component/qrcode | 1.1.1 | MIT | web-production-dependency |
| npm | @rc-component/rate | 1.0.1 | MIT | web-production-dependency |
| npm | @rc-component/resize-observer | 1.1.1 | MIT | web-production-dependency |
| npm | @rc-component/segmented | 1.3.0 | MIT | web-production-dependency |
| npm | @rc-component/select | 1.6.15 | MIT | web-production-dependency |
| npm | @rc-component/slider | 1.0.1 | MIT | web-production-dependency |
| npm | @rc-component/steps | 1.2.2 | MIT | web-production-dependency |
| npm | @rc-component/switch | 1.0.3 | MIT | web-production-dependency |
| npm | @rc-component/table | 1.9.1 | MIT | web-production-dependency |
| npm | @rc-component/tabs | 1.7.0 | MIT | web-production-dependency |
| npm | @rc-component/textarea | 1.1.2 | MIT | web-production-dependency |
| npm | @rc-component/tooltip | 1.4.0 | MIT | web-production-dependency |
| npm | @rc-component/tour | 2.3.0 | MIT | web-production-dependency |
| npm | @rc-component/tree | 1.2.4 | MIT | web-production-dependency |
| npm | @rc-component/tree-select | 1.8.0 | MIT | web-production-dependency |
| npm | @rc-component/trigger | 3.9.0 | MIT | web-production-dependency |
| npm | @rc-component/upload | 1.1.0 | MIT | web-production-dependency |
| npm | @rc-component/util | 1.10.0 | MIT | web-production-dependency |
| npm | @rc-component/virtual-list | 1.0.2 | MIT | web-production-dependency |
| npm | @rolldown/binding-android-arm64 | 1.0.0-rc.10 | MIT | build-or-optional-dependency |
| npm | @rolldown/binding-darwin-arm64 | 1.0.0-rc.10 | MIT | build-or-optional-dependency |
| npm | @rolldown/binding-darwin-x64 | 1.0.0-rc.10 | MIT | build-or-optional-dependency |
| npm | @rolldown/binding-freebsd-x64 | 1.0.0-rc.10 | MIT | build-or-optional-dependency |
| npm | @rolldown/binding-linux-arm-gnueabihf | 1.0.0-rc.10 | MIT | build-or-optional-dependency |
| npm | @rolldown/binding-linux-arm64-gnu | 1.0.0-rc.10 | MIT | build-or-optional-dependency |
| npm | @rolldown/binding-linux-arm64-musl | 1.0.0-rc.10 | MIT | build-or-optional-dependency |
| npm | @rolldown/binding-linux-ppc64-gnu | 1.0.0-rc.10 | MIT | build-or-optional-dependency |
| npm | @rolldown/binding-linux-s390x-gnu | 1.0.0-rc.10 | MIT | build-or-optional-dependency |
| npm | @rolldown/binding-linux-x64-gnu | 1.0.0-rc.10 | MIT | build-or-optional-dependency |
| npm | @rolldown/binding-linux-x64-musl | 1.0.0-rc.10 | MIT | build-or-optional-dependency |
| npm | @rolldown/binding-openharmony-arm64 | 1.0.0-rc.10 | MIT | build-or-optional-dependency |
| npm | @rolldown/binding-wasm32-wasi | 1.0.0-rc.10 | MIT | build-or-optional-dependency |
| npm | @rolldown/binding-win32-arm64-msvc | 1.0.0-rc.10 | MIT | build-or-optional-dependency |
| npm | @rolldown/binding-win32-x64-msvc | 1.0.0-rc.10 | MIT | build-or-optional-dependency |
| npm | @rolldown/pluginutils | 1.0.0-rc.10 | MIT | build-or-optional-dependency |
| npm | @rolldown/pluginutils | 1.0.0-rc.7 | MIT | build-or-optional-dependency |
| npm | @tailwindcss/node | 4.2.2 | MIT | build-or-optional-dependency |
| npm | @tailwindcss/oxide | 4.2.2 | MIT | build-or-optional-dependency |
| npm | @tailwindcss/oxide-android-arm64 | 4.2.2 | MIT | build-or-optional-dependency |
| npm | @tailwindcss/oxide-darwin-arm64 | 4.2.2 | MIT | build-or-optional-dependency |
| npm | @tailwindcss/oxide-darwin-x64 | 4.2.2 | MIT | build-or-optional-dependency |
| npm | @tailwindcss/oxide-freebsd-x64 | 4.2.2 | MIT | build-or-optional-dependency |
| npm | @tailwindcss/oxide-linux-arm-gnueabihf | 4.2.2 | MIT | build-or-optional-dependency |
| npm | @tailwindcss/oxide-linux-arm64-gnu | 4.2.2 | MIT | build-or-optional-dependency |
| npm | @tailwindcss/oxide-linux-arm64-musl | 4.2.2 | MIT | build-or-optional-dependency |
| npm | @tailwindcss/oxide-linux-x64-gnu | 4.2.2 | MIT | build-or-optional-dependency |
| npm | @tailwindcss/oxide-linux-x64-musl | 4.2.2 | MIT | build-or-optional-dependency |
| npm | @tailwindcss/oxide-wasm32-wasi | 4.2.2 | MIT | build-or-optional-dependency |
| npm | @tailwindcss/oxide-win32-arm64-msvc | 4.2.2 | MIT | build-or-optional-dependency |
| npm | @tailwindcss/oxide-win32-x64-msvc | 4.2.2 | MIT | build-or-optional-dependency |
| npm | @tailwindcss/postcss | 4.2.2 | MIT | build-or-optional-dependency |
| npm | @tanstack/query-core | 5.91.0 | MIT | web-production-dependency |
| npm | @tanstack/query-devtools | 5.93.0 | MIT | web-production-dependency |
| npm | @tanstack/react-query | 5.91.0 | MIT | web-production-dependency |
| npm | @tanstack/react-query-devtools | 5.91.3 | MIT | web-production-dependency |
| npm | @tybys/wasm-util | 0.10.1 | MIT | build-or-optional-dependency |
| npm | @types/estree | 1.0.8 | MIT | build-or-optional-dependency |
| npm | @types/json-schema | 7.0.15 | MIT | build-or-optional-dependency |
| npm | @types/node | 24.12.0 | MIT | build-or-optional-dependency |
| npm | @types/parse-json | 4.0.2 | MIT | web-production-dependency |
| npm | @types/react | 19.2.14 | MIT | web-production-dependency |
| npm | @types/react-dom | 19.2.3 | MIT | build-or-optional-dependency |
| npm | @typescript-eslint/eslint-plugin | 8.57.1 | MIT | build-or-optional-dependency |
| npm | @typescript-eslint/parser | 8.57.1 | MIT | build-or-optional-dependency |
| npm | @typescript-eslint/project-service | 8.57.1 | MIT | build-or-optional-dependency |
| npm | @typescript-eslint/scope-manager | 8.57.1 | MIT | build-or-optional-dependency |
| npm | @typescript-eslint/tsconfig-utils | 8.57.1 | MIT | build-or-optional-dependency |
| npm | @typescript-eslint/type-utils | 8.57.1 | MIT | build-or-optional-dependency |
| npm | @typescript-eslint/types | 8.57.1 | MIT | build-or-optional-dependency |
| npm | @typescript-eslint/typescript-estree | 8.57.1 | MIT | build-or-optional-dependency |
| npm | @typescript-eslint/utils | 8.57.1 | MIT | build-or-optional-dependency |
| npm | @typescript-eslint/visitor-keys | 8.57.1 | MIT | build-or-optional-dependency |
| npm | @umijs/route-utils | 4.0.3 | MIT | web-production-dependency |
| npm | @umijs/use-params | 1.0.9 | MIT | web-production-dependency |
| npm | @vitejs/plugin-react | 6.0.1 | MIT | build-or-optional-dependency |
| npm | @zip.js/zip.js | 2.8.60 | BSD-3-Clause | web-production-dependency |
| npm | acorn | 8.16.0 | MIT | build-or-optional-dependency |
| npm | acorn-jsx | 5.3.2 | MIT | build-or-optional-dependency |
| npm | agent-base | 6.0.2 | MIT | web-production-dependency |
| npm | ajv | 6.14.0 | MIT | build-or-optional-dependency |
| npm | ansi-styles | 4.3.0 | MIT | build-or-optional-dependency |
| npm | antd | 6.3.3 | MIT | web-production-dependency |
| npm | argparse | 2.0.1 | Python-2.0 | build-or-optional-dependency |
| npm | asynckit | 0.4.0 | MIT | web-production-dependency |
| npm | axios | 1.20.0 | MIT | web-production-dependency |
| npm | babel-plugin-macros | 3.1.0 | MIT | web-production-dependency |
| npm | balanced-match | 1.0.2 | MIT | build-or-optional-dependency |
| npm | balanced-match | 4.0.4 | MIT | build-or-optional-dependency |
| npm | baseline-browser-mapping | 2.10.8 | Apache-2.0 | build-or-optional-dependency |
| npm | brace-expansion | 1.1.12 | MIT | build-or-optional-dependency |
| npm | brace-expansion | 5.0.4 | MIT | build-or-optional-dependency |
| npm | browserslist | 4.28.1 | MIT | build-or-optional-dependency |
| npm | call-bind-apply-helpers | 1.0.2 | MIT | web-production-dependency |
| npm | callsites | 3.1.0 | MIT | web-production-dependency |
| npm | caniuse-lite | 1.0.30001780 | CC-BY-4.0 | build-or-optional-dependency |
| npm | chalk | 4.1.2 | MIT | build-or-optional-dependency |
| npm | clsx | 2.1.1 | MIT | web-production-dependency |
| npm | color-convert | 2.0.1 | MIT | build-or-optional-dependency |
| npm | color-name | 1.1.4 | MIT | build-or-optional-dependency |
| npm | combined-stream | 1.0.8 | MIT | web-production-dependency |
| npm | compute-scroll-into-view | 3.1.1 | MIT | web-production-dependency |
| npm | concat-map | 0.0.1 | MIT | build-or-optional-dependency |
| npm | convert-source-map | 1.9.0 | MIT | web-production-dependency |
| npm | convert-source-map | 2.0.0 | MIT | build-or-optional-dependency |
| npm | cookie | 1.1.1 | MIT | web-production-dependency |
| npm | cosmiconfig | 7.1.0 | MIT | web-production-dependency |
| npm | cross-spawn | 7.0.6 | MIT | build-or-optional-dependency |
| npm | csstype | 3.2.3 | MIT | web-production-dependency |
| npm | dayjs | 1.11.20 | MIT | web-production-dependency |
| npm | debug | 4.4.3 | MIT | web-production-dependency |
| npm | deep-is | 0.1.4 | MIT | build-or-optional-dependency |
| npm | delayed-stream | 1.0.0 | MIT | web-production-dependency |
| npm | dequal | 2.0.3 | MIT | web-production-dependency |
| npm | detect-libc | 2.1.2 | Apache-2.0 | build-or-optional-dependency |
| npm | dunder-proto | 1.0.1 | MIT | web-production-dependency |
| npm | echarts | 6.1.0 | Apache-2.0 | web-production-dependency |
| npm | electron-to-chromium | 1.5.321 | ISC | build-or-optional-dependency |
| npm | enhanced-resolve | 5.20.1 | MIT | build-or-optional-dependency |
| npm | error-ex | 1.3.4 | MIT | web-production-dependency |
| npm | es-define-property | 1.0.1 | MIT | web-production-dependency |
| npm | es-errors | 1.3.0 | MIT | web-production-dependency |
| npm | es-object-atoms | 1.1.2 | MIT | web-production-dependency |
| npm | es-set-tostringtag | 2.1.0 | MIT | web-production-dependency |
| npm | escalade | 3.2.0 | MIT | build-or-optional-dependency |
| npm | escape-string-regexp | 4.0.0 | MIT | web-production-dependency |
| npm | eslint | 9.39.4 | MIT | build-or-optional-dependency |
| npm | eslint-plugin-react-hooks | 7.0.1 | MIT | build-or-optional-dependency |
| npm | eslint-plugin-react-refresh | 0.5.2 | MIT | build-or-optional-dependency |
| npm | eslint-scope | 8.4.0 | BSD-2-Clause | build-or-optional-dependency |
| npm | eslint-visitor-keys | 3.4.3 | Apache-2.0 | build-or-optional-dependency |
| npm | eslint-visitor-keys | 4.2.1 | Apache-2.0 | build-or-optional-dependency |
| npm | eslint-visitor-keys | 5.0.1 | Apache-2.0 | build-or-optional-dependency |
| npm | espree | 10.4.0 | BSD-2-Clause | build-or-optional-dependency |
| npm | esquery | 1.7.0 | BSD-3-Clause | build-or-optional-dependency |
| npm | esrecurse | 4.3.0 | BSD-2-Clause | build-or-optional-dependency |
| npm | estraverse | 5.3.0 | BSD-2-Clause | build-or-optional-dependency |
| npm | esutils | 2.0.3 | BSD-2-Clause | build-or-optional-dependency |
| npm | fast-deep-equal | 3.1.3 | MIT | build-or-optional-dependency |
| npm | fast-json-stable-stringify | 2.1.0 | MIT | build-or-optional-dependency |
| npm | fast-levenshtein | 2.0.6 | MIT | build-or-optional-dependency |
| npm | fdir | 6.5.0 | MIT | build-or-optional-dependency |
| npm | file-entry-cache | 8.0.0 | MIT | build-or-optional-dependency |
| npm | find-root | 1.1.0 | MIT | web-production-dependency |
| npm | find-up | 5.0.0 | MIT | build-or-optional-dependency |
| npm | flat-cache | 4.0.1 | MIT | build-or-optional-dependency |
| npm | flatted | 3.4.2 | ISC | build-or-optional-dependency |
| npm | follow-redirects | 1.16.1 | MIT | web-production-dependency |
| npm | form-data | 4.0.6 | MIT | web-production-dependency |
| npm | framer-motion | 12.38.0 | MIT | web-production-dependency |
| npm | fsevents | 2.3.3 | MIT | build-or-optional-dependency |
| npm | function-bind | 1.1.2 | MIT | web-production-dependency |
| npm | gensync | 1.0.0-beta.2 | MIT | build-or-optional-dependency |
| npm | get-intrinsic | 1.3.0 | MIT | web-production-dependency |
| npm | get-proto | 1.0.1 | MIT | web-production-dependency |
| npm | glob-parent | 6.0.2 | ISC | build-or-optional-dependency |
| npm | globals | 14.0.0 | MIT | build-or-optional-dependency |
| npm | globals | 17.4.0 | MIT | build-or-optional-dependency |
| npm | gopd | 1.2.0 | MIT | web-production-dependency |
| npm | graceful-fs | 4.2.11 | ISC | build-or-optional-dependency |
| npm | has-flag | 4.0.0 | MIT | build-or-optional-dependency |
| npm | has-symbols | 1.1.0 | MIT | web-production-dependency |
| npm | has-tostringtag | 1.0.2 | MIT | web-production-dependency |
| npm | hash-wasm | 4.12.0 | MIT | web-production-dependency |
| npm | hasown | 2.0.4 | MIT | web-production-dependency |
| npm | hermes-estree | 0.25.1 | MIT | build-or-optional-dependency |
| npm | hermes-parser | 0.25.1 | MIT | build-or-optional-dependency |
| npm | https-proxy-agent | 5.0.1 | MIT | web-production-dependency |
| npm | ignore | 5.3.2 | MIT | build-or-optional-dependency |
| npm | ignore | 7.0.5 | MIT | build-or-optional-dependency |
| npm | import-fresh | 3.3.1 | MIT | web-production-dependency |
| npm | imurmurhash | 0.1.4 | MIT | build-or-optional-dependency |
| npm | is-arrayish | 0.2.1 | MIT | web-production-dependency |
| npm | is-core-module | 2.16.1 | MIT | web-production-dependency |
| npm | is-extglob | 2.1.1 | MIT | build-or-optional-dependency |
| npm | is-glob | 4.0.3 | MIT | build-or-optional-dependency |
| npm | is-mobile | 5.0.0 | MIT | web-production-dependency |
| npm | isexe | 2.0.0 | ISC | build-or-optional-dependency |
| npm | jiti | 2.6.1 | MIT | build-or-optional-dependency |
| npm | js-tokens | 4.0.0 | MIT | web-production-dependency |
| npm | js-yaml | 4.1.1 | MIT | build-or-optional-dependency |
| npm | jsesc | 3.1.0 | MIT | web-production-dependency |
| npm | json-buffer | 3.0.1 | MIT | build-or-optional-dependency |
| npm | json-parse-even-better-errors | 2.3.1 | MIT | web-production-dependency |
| npm | json-schema-traverse | 0.4.1 | MIT | build-or-optional-dependency |
| npm | json-stable-stringify-without-jsonify | 1.0.1 | MIT | build-or-optional-dependency |
| npm | json2mq | 0.2.0 | MIT | web-production-dependency |
| npm | json5 | 2.2.3 | MIT | build-or-optional-dependency |
| npm | keyv | 4.5.4 | MIT | build-or-optional-dependency |
| npm | levn | 0.4.1 | MIT | build-or-optional-dependency |
| npm | lightningcss | 1.32.0 | MPL-2.0 | build-or-optional-dependency |
| npm | lightningcss-android-arm64 | 1.32.0 | MPL-2.0 | build-or-optional-dependency |
| npm | lightningcss-darwin-arm64 | 1.32.0 | MPL-2.0 | build-or-optional-dependency |
| npm | lightningcss-darwin-x64 | 1.32.0 | MPL-2.0 | build-or-optional-dependency |
| npm | lightningcss-freebsd-x64 | 1.32.0 | MPL-2.0 | build-or-optional-dependency |
| npm | lightningcss-linux-arm-gnueabihf | 1.32.0 | MPL-2.0 | build-or-optional-dependency |
| npm | lightningcss-linux-arm64-gnu | 1.32.0 | MPL-2.0 | build-or-optional-dependency |
| npm | lightningcss-linux-arm64-musl | 1.32.0 | MPL-2.0 | build-or-optional-dependency |
| npm | lightningcss-linux-x64-gnu | 1.32.0 | MPL-2.0 | build-or-optional-dependency |
| npm | lightningcss-linux-x64-musl | 1.32.0 | MPL-2.0 | build-or-optional-dependency |
| npm | lightningcss-win32-arm64-msvc | 1.32.0 | MPL-2.0 | build-or-optional-dependency |
| npm | lightningcss-win32-x64-msvc | 1.32.0 | MPL-2.0 | build-or-optional-dependency |
| npm | lines-and-columns | 1.2.4 | MIT | web-production-dependency |
| npm | locate-path | 6.0.0 | MIT | build-or-optional-dependency |
| npm | lodash-es | 4.18.1 | MIT | web-production-dependency |
| npm | lodash.merge | 4.6.2 | MIT | build-or-optional-dependency |
| npm | loose-envify | 1.4.0 | MIT | web-production-dependency |
| npm | lru-cache | 5.1.1 | ISC | build-or-optional-dependency |
| npm | magic-string | 0.30.21 | MIT | build-or-optional-dependency |
| npm | math-intrinsics | 1.1.0 | MIT | web-production-dependency |
| npm | mime-db | 1.52.0 | MIT | web-production-dependency |
| npm | mime-types | 2.1.35 | MIT | web-production-dependency |
| npm | minimatch | 10.2.4 | BlueOak-1.0.0 | build-or-optional-dependency |
| npm | minimatch | 3.1.5 | ISC | build-or-optional-dependency |
| npm | monaco-editor | 0.52.2 | MIT | web-production-dependency |
| npm | motion-dom | 12.38.0 | MIT | web-production-dependency |
| npm | motion-utils | 12.36.0 | MIT | web-production-dependency |
| npm | ms | 2.1.3 | MIT | web-production-dependency |
| npm | nanoid | 3.3.11 | MIT | build-or-optional-dependency |
| npm | natural-compare | 1.4.0 | MIT | build-or-optional-dependency |
| npm | node-releases | 2.0.36 | MIT | build-or-optional-dependency |
| npm | object-assign | 4.1.1 | MIT | web-production-dependency |
| npm | optionator | 0.9.4 | MIT | build-or-optional-dependency |
| npm | p-limit | 3.1.0 | MIT | build-or-optional-dependency |
| npm | p-locate | 5.0.0 | MIT | build-or-optional-dependency |
| npm | parent-module | 1.0.1 | MIT | web-production-dependency |
| npm | parse-json | 5.2.0 | MIT | web-production-dependency |
| npm | path-exists | 4.0.0 | MIT | build-or-optional-dependency |
| npm | path-key | 3.1.1 | MIT | build-or-optional-dependency |
| npm | path-parse | 1.0.7 | MIT | web-production-dependency |
| npm | path-to-regexp | 8.4.2 | MIT | web-production-dependency |
| npm | path-type | 4.0.0 | MIT | web-production-dependency |
| npm | picocolors | 1.1.1 | ISC | web-production-dependency |
| npm | picomatch | 4.0.3 | MIT | build-or-optional-dependency |
| npm | postcss | 8.5.8 | MIT | build-or-optional-dependency |
| npm | prelude-ls | 1.2.1 | MIT | build-or-optional-dependency |
| npm | prop-types | 15.8.1 | MIT | web-production-dependency |
| npm | proxy-from-env | 2.1.0 | MIT | web-production-dependency |
| npm | punycode | 2.3.1 | MIT | build-or-optional-dependency |
| npm | rc-field-form | 2.7.1 | MIT | web-production-dependency |
| npm | rc-util | 5.44.4 | MIT | web-production-dependency |
| npm | react | 19.2.4 | MIT | web-production-dependency |
| npm | react-dom | 19.2.4 | MIT | web-production-dependency |
| npm | react-draggable | 4.5.0 | MIT | web-production-dependency |
| npm | react-is | 16.13.1 | MIT | web-production-dependency |
| npm | react-is | 18.3.1 | MIT | web-production-dependency |
| npm | react-router | 7.18.2 | MIT | web-production-dependency |
| npm | react-router-dom | 7.18.2 | MIT | web-production-dependency |
| npm | resolve | 1.22.11 | MIT | web-production-dependency |
| npm | resolve-from | 4.0.0 | MIT | web-production-dependency |
| npm | rolldown | 1.0.0-rc.10 | MIT | build-or-optional-dependency |
| npm | safe-stable-stringify | 2.5.0 | MIT | web-production-dependency |
| npm | scheduler | 0.27.0 | MIT | web-production-dependency |
| npm | scroll-into-view-if-needed | 3.1.0 | MIT | web-production-dependency |
| npm | semver | 6.3.1 | ISC | build-or-optional-dependency |
| npm | semver | 7.7.4 | ISC | build-or-optional-dependency |
| npm | set-cookie-parser | 2.7.2 | MIT | web-production-dependency |
| npm | shebang-command | 2.0.0 | MIT | build-or-optional-dependency |
| npm | shebang-regex | 3.0.0 | MIT | build-or-optional-dependency |
| npm | source-map | 0.5.7 | BSD-3-Clause | web-production-dependency |
| npm | source-map-js | 1.2.1 | BSD-3-Clause | build-or-optional-dependency |
| npm | string-convert | 0.2.1 | MIT | web-production-dependency |
| npm | strip-json-comments | 3.1.1 | MIT | build-or-optional-dependency |
| npm | stylis | 4.2.0 | MIT | web-production-dependency |
| npm | stylis | 4.3.6 | MIT | web-production-dependency |
| npm | supports-color | 7.2.0 | MIT | build-or-optional-dependency |
| npm | supports-preserve-symlinks-flag | 1.0.0 | MIT | web-production-dependency |
| npm | swr | 2.4.1 | MIT | web-production-dependency |
| npm | tailwindcss | 4.2.2 | MIT | web-production-dependency |
| npm | tapable | 2.3.0 | MIT | build-or-optional-dependency |
| npm | throttle-debounce | 5.0.2 | MIT | web-production-dependency |
| npm | tinyglobby | 0.2.15 | MIT | build-or-optional-dependency |
| npm | ts-api-utils | 2.5.0 | MIT | build-or-optional-dependency |
| npm | tslib | 2.3.0 | 0BSD | web-production-dependency |
| npm | tslib | 2.8.1 | 0BSD | web-production-dependency |
| npm | type-check | 0.4.0 | MIT | build-or-optional-dependency |
| npm | typescript | 5.9.3 | Apache-2.0 | build-or-optional-dependency |
| npm | typescript-eslint | 8.57.1 | MIT | build-or-optional-dependency |
| npm | undici-types | 7.16.0 | MIT | build-or-optional-dependency |
| npm | update-browserslist-db | 1.2.3 | MIT | build-or-optional-dependency |
| npm | uri-js | 4.4.1 | BSD-2-Clause | build-or-optional-dependency |
| npm | use-sync-external-store | 1.6.0 | MIT | web-production-dependency |
| npm | vite | 8.0.1 | MIT | build-or-optional-dependency |
| npm | which | 2.0.2 | ISC | build-or-optional-dependency |
| npm | word-wrap | 1.2.5 | MIT | build-or-optional-dependency |
| npm | yallist | 3.1.1 | ISC | build-or-optional-dependency |
| npm | yaml | 1.10.3 | ISC | web-production-dependency |
| npm | yaml | 2.9.0 | ISC | web-production-dependency |
| npm | yocto-queue | 0.1.0 | MIT | build-or-optional-dependency |
| npm | zod | 4.3.6 | MIT | build-or-optional-dependency |
| npm | zod-validation-error | 4.0.2 | MIT | build-or-optional-dependency |
| npm | zrender | 6.1.0 | BSD-3-Clause | web-production-dependency |
| npm | zustand | 4.5.7 | MIT | web-production-dependency |
