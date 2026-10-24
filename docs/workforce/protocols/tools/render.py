#!/usr/bin/env python3
"""Render source-extracted HTTP/type/SDK references. No services or credentials."""
import ast
import hashlib
import json
import re
import sys
from pathlib import Path
ROOT = Path(__file__).resolve().parents[4]
OUT = ROOT / 'docs/workforce/protocols'
x = json.loads(Path(sys.argv[1]).read_text())
T = {v['Name']: v for v in x['types']}

def anchor(s):
    return 't-' + hashlib.sha256(s.encode()).hexdigest()[:16]

def source(s):
    if not s: return '源码类型（由 Go 编译器导出）'
    p, sep, line = s.rpartition(':')
    return f'[{p}:{line}](../../../{p}#L{line})' if sep else f'[源码](../../../{s})'

def esc(s): return str(s).replace('|', '\\|').replace('\n', ' ')

def link(n):
    if n in T: return f'[`{esc(n)}`](wire-types.md#{anchor(n)})'
    if n.startswith('[]') and n[2:] in T: return 'array<' + link(n[2:]) + '>'
    if n.startswith('*') and n[1:] in T: return link(n[1:]) + ' 或 null'
    return '`' + esc(n) + '`'

def wire(t, tag='', http=False):
    nullable = t.startswith('*')
    if nullable: t=t[1:]
    if t == 'json.RawMessage' or t == 'any' or t.startswith('interface'): w='任意 JSON 值'
    elif t == '[]uint8' or t == '[]byte': w='string(base64)'
    elif t.startswith('[]'): w='array<' + wire(t[2:],tag,http) + '>'
    elif t.startswith('map['): w='object<string, ' + wire(t.split(']',1)[1],tag,http) + '>'
    elif t in ('time.Time',): w='string(RFC3339/RFC3339Nano)'
    elif t in ('int64','uint64'): w='string(decimal)' if ',string' in tag or http else 'integer'
    elif t.startswith(('int','uint')): w='integer'
    elif t.startswith('float'): w='number'
    elif t == 'bool': w='boolean'
    elif t == 'string': w='string'
    elif t.endswith('transport.Int64Slice'): w='array<string(decimal)>'
    elif t in T and T[t]['Kind']=='value': w=wire(T[t]['Underlying'],tag,http)
    elif t in T: w=link(t)
    elif t.startswith('struct{'): w='object（见 Go 类型或匿名结构索引）'
    else: w='`'+esc(t)+'`'
    return w + (' 或 null' if nullable else '')

def fields(n, seen=None):
    seen=set(seen or ())
    if n in seen or n not in T:return []
    seen.add(n); result=[]
    for f in T[n].get('Fields') or []:
        if f['Embedded'] and not re.search(r'json:"[^,\"]',f['Tag']) and f['Reference']:
            result += fields(f['Reference'],seen)
        else: result.append(f)
    return result

def table(n, http=False):
    fs=fields(n)
    if not fs:return '类型：'+link(n)+'。\n'
    rows=['| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |','| --- | --- | --- | --- |']
    for f in fs:
        note=[]
        if 'omitempty' in f['Tag']:note.append('零值可省略')
        if ',string' in f['Tag']:note.append('JSON 引号编码')
        if f['Reference']:note.append(link(f['Reference']))
        note.append('Go: `'+esc(f['GoType'])+'`')
        rows.append('| `'+esc(f['Name'])+'` | '+wire(f['GoType'],f['Tag'])+' | '+wire(f['GoType'],f['Tag'],True)+' | '+'；'.join(note)+' |')
    return '\n'.join(rows)+'\n'

# Dynamic query helper arguments are not literal Query calls in the AST.
for r in x['routes']:
    if r['Handler'] in ('list','listProjectTasks') and '/task/controller/' in r['Source']:
        for n in ('from','to'):
            r['Parameters'].append({'In':'query','Name':n,'Type':'string(RFC3339 或 YYYY-MM-DD)'})
    if '/project/controller/' in r['Source']:
        src=(ROOT/r['Source'].split(':')[0]).read_text()
        # Methods invoking optionalQueryID pass an explicit parameter name.
        pos=src.find('func (h *Handler) '+r['Handler']+'(')
        block=src[pos:src.find('\nfunc ',pos+1)] if pos>=0 else ''
        for n in re.findall(r'optionalQueryID\(c, "([^"]+)"\)',block):
            r['Parameters'].append({'In':'query','Name':n,'Type':'string(decimal ID)'})

raw='/api/internal/workforce/v1/model-invocations/'
def group(r):
    p=r['Path']
    if p.startswith('/api/open/'):return 'open'
    if p.startswith(raw):return 'provider'
    if p.startswith('/api/internal/'):return 'worker'
    return 'operator'
intro={
'open':('Open REST 方法参考','外部系统使用 Application API Key。接入步骤、必填与约束见 [Open REST 指南](open-rest.md)。'),
'operator':('Web／管理 REST 方法参考','使用 Framework 用户身份、权限与组织范围。权限列为空表示包装器未给出字面权限，不能解释为匿名开放。详见 [公共约定](common.md)。'),
'worker':('Worker Machine HTTP 方法参考','只供 Worker／Control Plane 使用 Worker Bearer credential；不接受 Application API Key 代替 Worker 身份。详见 [Worker 与队列指南](worker-amqp.md)。'),
'provider':('Provider Invocation HTTP 方法参考','只供受租约约束的 Agent Attempt／ReviewRun。详见 [模型网关指南](provider-gateway.md)。')}
for g,(title,desc) in intro.items():
    routes=[r for r in x['routes'] if group(r)==g]
    out=[f'# {title}\n',f'来源：随附独立源码快照（结构经检查）。共 **{len(routes)}** 个方法。{desc}\n',
         '此页逐项列出方法、URL、路径／查询／Header／表单参数、请求体字段、响应类型和实现位置。嵌套对象点进类型索引；`omitempty` 仅是编码规则，业务必填与状态前置条件由对应应用／域校验决定。各接口并不统一采用 PATCH 或自动幂等。\n',
         '| 方法 | 路径 | Handler |\n| --- | --- | --- |']
    for i,r in enumerate(routes):out.append(f'| {r["Method"]} | [`{r["Path"]}`](#route-{i+1}) | `{r["Handler"]}` |')
    for i,r in enumerate(routes):
        out += [f'\n<a id="route-{i+1}"></a>\n\n## {r["Method"]} `{r["Path"]}`\n',f'实现：{source(r["Source"])}。注册：{source(r["Registration"])}。',
                f'\n权限：`{r["Permission"]}`。\n' if r['Permission'] else '\n鉴权：遵循本页协议身份与公共约定；本方法未提取到字面 RBAC 权限。\n']
        if r['Parameters']:
            out += ['| 位置 | 参数 | 传输类型 |\n| --- | --- | --- |']
            for p in r['Parameters']:out.append(f'| {p["In"]} | `{p["Name"]}` | {esc(p["Type"])} |')
        else:out.append('路径／查询／表单：未声明额外参数；鉴权 Header 见指南。')
        out.append('\n### 请求体\n')
        if r['Requests']:
            for n in r['Requests']:out += [link(n)+'\n',table(n)]
        elif g=='provider':out.append('原始 Provider JSON object（根据 URL 对应 Responses 或 Messages 操作），网关按字节计算 SHA256；字段兼容性由已配置 Provider 决定。')
        elif r['Path'].endswith('/archive-import'):out.append('原始二进制 tar.gz archive body（不是 JSON 或 multipart）；最大 8 MiB。query expectedRevision:string(decimal int64) 用于版本检查，具体必填校验见该方法源码。')
        elif any(p['In'] in ('file','form') for p in r['Parameters']):out.append('`multipart/form-data`，字段见上述表单参数；不是 JSON。')
        else:out.append('没有已声明的业务 JSON DTO；按照实现使用空请求体。状态操作仍需满足当前业务状态及权限。')
        out.append('\n### 响应\n')
        if g=='provider':out.append('原始上游 HTTP status、JSON 或 SSE body；成功路径不套 `{code,data,message}`。网关拒绝／失败格式见指南。')
        elif r['Responses'] or r['ResponseExpressions']:
            out.append('默认包装为 `Result.data`；`data` 类型：'+ ' / '.join(link(n) for n in r['Responses'])+'。')
            if r['ResponseExpressions']:
                out += ['\n直接构造的 `data` 字段：\n','| 字段 | Go 类型与 HTTP 传输类型 |\n| --- | --- |']
                for expr in r['ResponseExpressions']:
                    k,t=expr.split(': ',1);out.append('| `'+k+'` | '+wire(t,http=True)+'；Go `'+esc(t)+'` |')
        elif '/download' in r['Path']:out.append('成功为二进制文件流；错误在开始发送之前返回 HTTP 错误，开始之后可能中止流。请核对大小与 SHA256。')
        elif r['Handler']=='eventStream':out.append('`text/event-stream`，见 [SSE 协议](sse.md)。')
        elif r['Handler']=='finalizationRuns':out.append('`data` 为后处理提供方的 finalization run 数组；提供方未配置时为 `[]`。入口以 `any` 声明，实际类型见实现，不保证静态统一 DTO。')
        else:out.append('响应由实现内的包装器构造，参见上方源码位置。')
        if r['ErrorCodes']:out.append('\n入口／同包辅助方法中可定位的 reasonCode：'+', '.join('`'+c+'`' for c in r['ErrorCodes'])+'。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。')
    (OUT/f'http-{g}.md').write_text('\n'.join(out)+'\n')
out=['# Wire 类型索引\n','来源：当前 Go 类型与 JSON tag。此页是字段级参考，类型名的包路径保留用于区分同名 DTO；不是要求客户端使用这些 Go 包。HTTP `response.Success` 对 int64/uint64 做字符串归一化；原生 JSON、消息、manifest 不经过该归一化，遵循 `json:",string"` 与自定义序列化。原生 JSON 列不等于每个字段都可直接从请求提交。\n','`*T` 可为 null；slice 在原生 JSON 可为 null；`omitempty` 是省略零值规则，不代表业务可选。未声明结构的 `RawMessage`／`map[string]any` 仍然是开放 JSON，需读对应协议语义。嵌入对象按未命名 JSON 嵌入规则展开；自定义 MarshalJSON 见 [公共约定](common.md)。\n']
for n,s in sorted(T.items()):
    out += [f'<a id="{anchor(n)}"></a>\n\n## `{n}`\n',source(s['Source'])+'\n']
    if s['Kind']=='value':out.append('底层类型：`'+esc(s['Underlying'])+'`。')
    else:out.append(table(n))
    if s.get('Enum'):out.append('声明的常量值：'+', '.join('`'+esc(v)+'`' for v in s['Enum'])+'。')
(OUT/'wire-types.md').write_text('\n'.join(out)+'\n')
# SDK is static AST inspection: do not import or execute adapters.
out=['# Python Runtime SDK 方法与类型\n','源码位于 `deploy/workforce/runtime-sdk/src/seven_workforce_runtime`。以下签名和 dataclass 字段从 AST 提取，不执行 SDK，也不调用 Provider。未写类型注解的参数明确标注；不能把推断当作接口声明。生命周期与 wire 映射见 [Agent Adapter](agent-adapter.md) 和 [Result Processor](result-processor.md)。\n']
files=['agent.py','interaction.py','tools.py','processor/__init__.py']
for fn in files:
    path=ROOT/'deploy/workforce/runtime-sdk/src/seven_workforce_runtime'/fn
    tree=ast.parse(path.read_text());out.append('## 模块 `'+fn.replace('/__init__.py','').replace('.py','')+'`\n')
    for node in tree.body:
        if isinstance(node,ast.Assign) and any(isinstance(t,ast.Name) and t.id=='Handler' for t in node.targets):out.append('回调别名：`'+esc(ast.unparse(node))+'`。\n')
        nodes=[]
        if isinstance(node,ast.ClassDef) and not node.name.startswith('_'):
            out.append('### `'+node.name+'`\n')
            fields=[v for v in node.body if isinstance(v,ast.AnnAssign) and isinstance(v.target,ast.Name) and not v.target.id.startswith('_')]
            if fields:
                out.append('| 字段 | Python 类型 | 默认值 |\n| --- | --- | --- |')
                for v in fields:out.append('| `'+v.target.id+'` | `'+esc(ast.unparse(v.annotation))+'` | '+('`'+esc(ast.unparse(v.value))+'`' if v.value else '构造时提供')+' |')
            if any(isinstance(b,ast.Attribute) and b.attr=='Enum' for b in node.bases):
                for v in node.body:
                    if isinstance(v,ast.Assign):out.append('`'+esc(ast.unparse(v))+'`')
            nodes=[(v,node.name+'.') for v in node.body if isinstance(v,(ast.FunctionDef,ast.AsyncFunctionDef)) and (not v.name.startswith('_') or v.name=='__init__')]
        elif isinstance(node,(ast.FunctionDef,ast.AsyncFunctionDef)) and not node.name.startswith('_'):nodes=[(node,'')]
        for v,prefix in nodes:
            returns=ast.unparse(v.returns) if v.returns else '未注解（以实现为准）'
            out += ['\n#### `'+prefix+v.name+'`\n',f'来源：{source(str(path.relative_to(ROOT))+":"+str(v.lineno))}。'+('异步方法，需 `await`。' if isinstance(v,ast.AsyncFunctionDef) else '同步方法。'),
                    '\n```python\n'+('async ' if isinstance(v,ast.AsyncFunctionDef) else '')+'def '+prefix+v.name+'('+ast.unparse(v.args)+') -> '+returns+'\n```\n','| 参数 | Python 类型 | 默认／调用要求 |\n| --- | --- | --- |']
            args=v.args.posonlyargs+v.args.args;defs=[None]*(len(args)-len(v.args.defaults))+v.args.defaults
            entries=list(zip(args,defs))+list(zip(v.args.kwonlyargs,v.args.kw_defaults))
            for a,d in entries:
                if a.arg in ('self','cls'):continue
                out.append('| `'+a.arg+'` | `'+esc(ast.unparse(a.annotation) if a.annotation else '未注解')+'` | '+('必传' if d is None else '`'+esc(ast.unparse(d))+'`')+('；仅关键字参数' if a in v.args.kwonlyargs else '')+' |')
            for a in (v.args.vararg,v.args.kwarg):
                if a:out.append('| `'+a.arg+'` | `'+esc(ast.unparse(a.annotation) if a.annotation else '未注解')+'` | 可变参数 |')
            doc=ast.get_docstring(v)
            if doc:out.append('\n'+doc+'\n')
            if any(isinstance(d,ast.Name) and d.id=='property' for d in v.decorator_list):out.append('这是 `@property` 属性，读取时不加括号。')
out.append('''
## 无注解入口的实现约束补充

下表是从实现核对的可接入类型，区别于上面原始签名的类型注解。

| 入口／参数 | 接入类型与返回 |
| --- | --- |
| AgentAdapter.__init__.capabilities | tuple/list/set/frozenset[str]，仅可选择interaction.request或tool.request；默认空tuple |
| AgentAdapter.capabilities | property，返回list[str]；不加括号调用 |
| AgentAdapter.run/resume/message/suspend | 传入具有两个位置参数的async handler；注册返回原handler，可作decorator |
| AgentAdapter.serve_streams | reader需async readline()->bytes；writer需write(bytes)和async drain()；serve封装标准流 |
| RoundResult.finished.summary/session_reference | str，默认空；返回RoundResult |
| 各dataclass.wire | 返回dict[str,JSON]，wire字段见Agent协议；Python字段名可能使用snake_case |
| QuestionRequest.from_wire.value | dict含interactionKey:str、questions:list[Question wire]、isBlocking:bool；返回QuestionRequest，roundKey由ExecutionContext绑定 |
| ToolTarget/ToolCallRequest/ToolResult.from_wire.value | 对应wire dict，返回对应dataclass |
| interaction.key/question_key.value | str；验证通过返回None，失败ProtocolError |
| interaction.text | value:str、limit:int（bytes）、nonempty:bool；返回None |
| interaction.fields | value:dict、expected:str（空格分隔字段名）；返回None |
| interaction.validate_payload / tools.validate_payload | method:str、payload:dict；校验与method匹配的闭合字段 |
| Processor helper的checks/findings/questions/outputs/actions | list[对应ResultContract项]或None；字段定义见Result Processor指南 |

ExecutionContext由平台按round创建，emit是异步frame发送回调，capabilities是当前已准入能力集合。常规Adapter作者使用handler收到的context，不自行构造另一个有独立round的context。
''')
(OUT/'python-sdk.md').write_text('\n'.join(out)+'\n')
# Keep an inspectable machine-readable inventory, with manual query discoveries included.
(OUT/'contracts.json').write_text(json.dumps(x,ensure_ascii=False,indent=2)+'\n')
print('Rendered',len(x['routes']),'HTTP methods;',len(T),'Go types; SDK AST reference')
# CLI flags are read only as source text; environment defaults are labels, never values.
out=['# CLI 命令与参数\n','用户工具 `seven-workerctl` 使用Framework用户token调用operator API；镜像内 `seven-worker-cli` 使用冻结spec与Supervisor socket，二者权限与用途不同。以下只解析源码，不执行命令。\n',
'''## seven-workerctl

| 命令 | 参数与类型 | 对应行为 |
| --- | --- | --- |
| version | 无 | 本地版本 |
| health | --server:string；--timeout:duration=5s | GET /api/healthz |
| task create | 参数表runTask，Project/模型/Runtime/instruction必需 | POST /api/workforce/projects/:id/tasks |
| task list | --limit:integer=50，加公共flags | GET tasks |
| task get / events / cancel | task-id:string位置参数，加公共flags | GET task / GET events / POST cancel |
| artifact list | task-id:string位置参数，加公共flags | GET artifacts |
| artifact download | --output:string必传；task-id/artifact-id:string位置参数 | 下载文件，落地0600 |
| help | 无 | usage |

公共flags为--server:string（源码默认http://127.0.0.1:9277，部署覆盖）、--token:string（Framework access token）、--timeout:duration=30s。优先用SEVEN_ACCESS_TOKEN注入，避免命令行明文。flag解析在首个位置参数停止，因此flags应放在task-id等位置参数之前；artifact download源码usage将--output放在最后不等于该排序可用。

## seven-worker-cli

| 命令 | 参数形态／类型 | 用途 |
| --- | --- | --- |
| version | 无 | image版本 |
| image probe --json | 固定形态 | seven.worker-image/v1准入探测 |
| supervise | --spec:string（或环境提供路径） | 运行冻结AttemptSpec |
| check | --input:string必传；其余见runCheck flags | 冻结completion check |
| review | --spec:string，见reviewMain | 独立Review执行 |
| result-fixture / result-deliver | --spec:string | 受保护fixture／delivery spec |
| hook dispatch | --event:string | 已批准Hook事件 |
| control | [--stream]，stdin JSON request | Supervisor UDS控制 |
| interaction ask --request FILE | FILE:string必需 | 提问；等待正常答案回执 |
| status / interrupt / diagnostics | 当前socket与冻结identity | Supervisor控制 |
| review-provider-relay | flags见fakeReviewProviderRelay | image测试／受控relay，不是外部用户接口 |
| delivery-egress-gateway / target-egress-gateway / delivery-egress-probe-target | flags见对应源码表 | egress隔离与测试辅助 |

spec路径、socket路径是string；里面承载JSON对象需遵守 [类型索引](wire-types.md) 与 [Supervisor](supervisor.md)。stdin/stdout与exit code的含义由具体命令决定，不能把进程成功当作Task成功；Result Processor候选与CompletionProposal仍需独立核验。

## 从源码提取的flag定义

下表按实际解析函数给出flag名、类型、源码默认表达式与描述。默认表达式中os.Getenv只表示环境名，不读取其值。相同函数内不同subcommand不一定接受同一组flag，按上方命令分组与源码case使用。
''']
for dirname in ('seven-workerctl','seven-worker-cli'):
    for path in sorted((ROOT/'seven-agent-force-server/cmd'/dirname).glob('*.go')):
        if path.name.endswith('_test.go'):continue
        text=path.read_text()
        functions=list(re.finditer(r'^func (?:\([^\n]+\) )?([\w]+)\(',text,re.M))
        for i,f in enumerate(functions):
            block=text[f.start():functions[i+1].start() if i+1<len(functions) else len(text)]
            flags=[]
            pattern=r'\b\w+\.(String|Int|Int64|Bool|Duration)(Var)?\((?:[^,\n]+,\s*)?"([^"\n]+)",\s*(.+?),\s*"([^"\n]*)"\)'
            for m in re.finditer(pattern,block):
                typ,_,n,default,desc=m.groups();flags.append((n,{'String':'string','Int':'integer','Int64':'int64','Bool':'boolean','Duration':'duration'}[typ],default,desc))
            for m in re.finditer(r'\b\w+\.Var\([^,]+,\s*"([^"]+)",\s*"([^"]+)"\)',block):flags.append((m[1],'repeatable string','[]',m[2]))
            if flags:
                out += [f'### `{dirname}.{f[1]}`\n',source(str(path.relative_to(ROOT))+':'+str(text[:f.start()].count('\n')+1))+'\n','| flag | 类型 | 默认表达式 | 说明 |\n| --- | --- | --- | --- |']
                for n,t,d,desc in flags:out.append(f'| `--{n}` | {t} | `{esc(d)}` | {esc(desc)} |')
(OUT/'cli.md').write_text('\n'.join(out)+'\n')
