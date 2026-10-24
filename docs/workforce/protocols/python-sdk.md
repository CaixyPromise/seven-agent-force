# Python Runtime SDK 方法与类型

源码位于 `deploy/workforce/runtime-sdk/src/seven_agent/force`。以下签名和 dataclass 字段从 AST 提取，不执行 SDK，也不调用 Provider。未写类型注解的参数明确标注；不能把推断当作接口声明。生命周期与 wire 映射见 [Agent Adapter](agent-adapter.md) 和 [Result Processor](result-processor.md)。

## 模块 `agent`

### `RoundOutcome`

`FINISHED = 'FINISHED'`
`CANCELLED = 'CANCELLED'`
`FAILED = 'FAILED'`
### `ActivityStatus`

`STARTED = 'STARTED'`
`COMPLETED = 'COMPLETED'`
`SUCCEEDED = 'SUCCEEDED'`
`FAILED = 'FAILED'`
### `RunRequest`

| 字段 | Python 类型 | 默认值 |
| --- | --- | --- |
| `round_key` | `str` | 构造时提供 |
| `inputs` | `dict` | 构造时提供 |
### `ResumeRequest`

| 字段 | Python 类型 | 默认值 |
| --- | --- | --- |
| `session_reference` | `str` | 构造时提供 |
### `RoundResult`

| 字段 | Python 类型 | 默认值 |
| --- | --- | --- |
| `outcome` | `RoundOutcome` | 构造时提供 |
| `summary` | `str` | `''` |
| `session_reference` | `str` | `''` |

#### `RoundResult.finished`

来源：[deploy/workforce/runtime-sdk/src/seven_agent/force/agent.py:53](../../../deploy/workforce/runtime-sdk/src/seven_agent/force/agent.py#L53)。同步方法。

```python
def RoundResult.finished(cls, *, summary='', session_reference='') -> 未注解（以实现为准）
```

| 参数 | Python 类型 | 默认／调用要求 |
| --- | --- | --- |
| `summary` | `未注解` | `''`；仅关键字参数 |
| `session_reference` | `未注解` | `''`；仅关键字参数 |
### `ExecutionContext`


#### `ExecutionContext.__init__`

来源：[deploy/workforce/runtime-sdk/src/seven_agent/force/agent.py:58](../../../deploy/workforce/runtime-sdk/src/seven_agent/force/agent.py#L58)。同步方法。

```python
def ExecutionContext.__init__(self, round_key, emit, *, capabilities=()) -> 未注解（以实现为准）
```

| 参数 | Python 类型 | 默认／调用要求 |
| --- | --- | --- |
| `round_key` | `未注解` | 必传 |
| `emit` | `未注解` | 必传 |
| `capabilities` | `未注解` | `()`；仅关键字参数 |

#### `ExecutionContext.received_response`

来源：[deploy/workforce/runtime-sdk/src/seven_agent/force/agent.py:76](../../../deploy/workforce/runtime-sdk/src/seven_agent/force/agent.py#L76)。同步方法。

```python
def ExecutionContext.received_response(self, interaction_key: str) -> InteractionResponse | None
```

| 参数 | Python 类型 | 默认／调用要求 |
| --- | --- | --- |
| `interaction_key` | `str` | 必传 |

Snapshot accepted delivery even before the ask coroutine is scheduled.

This is local receipt, not persistence or application. Suspend handlers
must persist this snapshot themselves before claiming native quiescence.


#### `ExecutionContext.ask`

来源：[deploy/workforce/runtime-sdk/src/seven_agent/force/agent.py:113](../../../deploy/workforce/runtime-sdk/src/seven_agent/force/agent.py#L113)。异步方法，需 `await`。

```python
async def ExecutionContext.ask(self, request: QuestionRequest) -> tuple[InteractionAnswer, ...]
```

| 参数 | Python 类型 | 默认／调用要求 |
| --- | --- | --- |
| `request` | `QuestionRequest` | 必传 |

Await the single pending keyed answer; same-key callers share it.


#### `ExecutionContext.receive_restored_answer`

来源：[deploy/workforce/runtime-sdk/src/seven_agent/force/agent.py:125](../../../deploy/workforce/runtime-sdk/src/seven_agent/force/agent.py#L125)。异步方法，需 `await`。

```python
async def ExecutionContext.receive_restored_answer(self, question: QuestionRequest, expected_response: InteractionResponse) -> tuple[InteractionAnswer, ...]
```

| 参数 | Python 类型 | 默认／调用要求 |
| --- | --- | --- |
| `question` | `QuestionRequest` | 必传 |
| `expected_response` | `InteractionResponse` | 必传 |

Wait for normal delivery of a saved response in this new round.

Registers no visible request and generates no automatic receipt. Only a
matching interaction.respond can resolve the wait and produce RECEIVED.
The private snapshot alone is not accepted delivery or authority.


#### `ExecutionContext.started`

来源：[deploy/workforce/runtime-sdk/src/seven_agent/force/agent.py:156](../../../deploy/workforce/runtime-sdk/src/seven_agent/force/agent.py#L156)。异步方法，需 `await`。

```python
async def ExecutionContext.started(self, *, native_pid: int | None=None) -> 未注解（以实现为准）
```

| 参数 | Python 类型 | 默认／调用要求 |
| --- | --- | --- |
| `native_pid` | `int \| None` | `None`；仅关键字参数 |

Call after execution starts; omit PID only for an in-process agent.


#### `ExecutionContext.emit_message`

来源：[deploy/workforce/runtime-sdk/src/seven_agent/force/agent.py:168](../../../deploy/workforce/runtime-sdk/src/seven_agent/force/agent.py#L168)。异步方法，需 `await`。

```python
async def ExecutionContext.emit_message(self, text: str) -> 未注解（以实现为准）
```

| 参数 | Python 类型 | 默认／调用要求 |
| --- | --- | --- |
| `text` | `str` | 必传 |

#### `ExecutionContext.emit_plan`

来源：[deploy/workforce/runtime-sdk/src/seven_agent/force/agent.py:173](../../../deploy/workforce/runtime-sdk/src/seven_agent/force/agent.py#L173)。异步方法，需 `await`。

```python
async def ExecutionContext.emit_plan(self, steps: list[str]) -> 未注解（以实现为准）
```

| 参数 | Python 类型 | 默认／调用要求 |
| --- | --- | --- |
| `steps` | `list[str]` | 必传 |

#### `ExecutionContext.emit_activity`

来源：[deploy/workforce/runtime-sdk/src/seven_agent/force/agent.py:178](../../../deploy/workforce/runtime-sdk/src/seven_agent/force/agent.py#L178)。异步方法，需 `await`。

```python
async def ExecutionContext.emit_activity(self, *, key: str, title: str, status: ActivityStatus) -> 未注解（以实现为准）
```

| 参数 | Python 类型 | 默认／调用要求 |
| --- | --- | --- |
| `key` | `str` | 必传；仅关键字参数 |
| `title` | `str` | 必传；仅关键字参数 |
| `status` | `ActivityStatus` | 必传；仅关键字参数 |

#### `ExecutionContext.tool_catalog_ready`

来源：[deploy/workforce/runtime-sdk/src/seven_agent/force/agent.py:184](../../../deploy/workforce/runtime-sdk/src/seven_agent/force/agent.py#L184)。异步方法，需 `await`。

```python
async def ExecutionContext.tool_catalog_ready(self, tool_names: tuple[str, ...]) -> 未注解（以实现为准）
```

| 参数 | Python 类型 | 默认／调用要求 |
| --- | --- | --- |
| `tool_names` | `tuple[str, ...]` | 必传 |

Report the safe tool-name catalog accepted by the native runtime.


#### `ExecutionContext.close_interaction`

来源：[deploy/workforce/runtime-sdk/src/seven_agent/force/agent.py:193](../../../deploy/workforce/runtime-sdk/src/seven_agent/force/agent.py#L193)。异步方法，需 `await`。

```python
async def ExecutionContext.close_interaction(self, interaction_key: str, *, reason: str) -> 未注解（以实现为准）
```

| 参数 | Python 类型 | 默认／调用要求 |
| --- | --- | --- |
| `interaction_key` | `str` | 必传 |
| `reason` | `str` | 必传；仅关键字参数 |

Close a native request that can no longer accept a response.


#### `ExecutionContext.request_tool`

来源：[deploy/workforce/runtime-sdk/src/seven_agent/force/agent.py:226](../../../deploy/workforce/runtime-sdk/src/seven_agent/force/agent.py#L226)。异步方法，需 `await`。

```python
async def ExecutionContext.request_tool(self, request: ToolCallRequest) -> ToolResult
```

| 参数 | Python 类型 | 默认／调用要求 |
| --- | --- | --- |
| `request` | `ToolCallRequest` | 必传 |

Publish one keyed tool call and wait for the platform-owned result.


#### `ExecutionContext.wait_for_tool_result`

来源：[deploy/workforce/runtime-sdk/src/seven_agent/force/agent.py:237](../../../deploy/workforce/runtime-sdk/src/seven_agent/force/agent.py#L237)。异步方法，需 `await`。

```python
async def ExecutionContext.wait_for_tool_result(self, local_tool_call_key: str) -> ToolResult
```

| 参数 | Python 类型 | 默认／调用要求 |
| --- | --- | --- |
| `local_tool_call_key` | `str` | 必传 |

Wait for an already-published tool request without publishing again.


#### `ExecutionContext.tool_received`

来源：[deploy/workforce/runtime-sdk/src/seven_agent/force/agent.py:244](../../../deploy/workforce/runtime-sdk/src/seven_agent/force/agent.py#L244)。异步方法，需 `await`。

```python
async def ExecutionContext.tool_received(self, result: ToolResult, *, status=ToolDeliveryStatus.RECEIVED) -> 未注解（以实现为准）
```

| 参数 | Python 类型 | 默认／调用要求 |
| --- | --- | --- |
| `result` | `ToolResult` | 必传 |
| `status` | `未注解` | `ToolDeliveryStatus.RECEIVED`；仅关键字参数 |

Report native receipt after the downstream Agent accepted the result.


#### `ExecutionContext.close_tool`

来源：[deploy/workforce/runtime-sdk/src/seven_agent/force/agent.py:258](../../../deploy/workforce/runtime-sdk/src/seven_agent/force/agent.py#L258)。异步方法，需 `await`。

```python
async def ExecutionContext.close_tool(self, local_tool_call_key: str, *, reason: str) -> 未注解（以实现为准）
```

| 参数 | Python 类型 | 默认／调用要求 |
| --- | --- | --- |
| `local_tool_call_key` | `str` | 必传 |
| `reason` | `str` | 必传；仅关键字参数 |
回调别名：`Handler = Callable[[RunRequest, ExecutionContext], Awaitable[RoundResult]]`。

### `AgentAdapter`


#### `AgentAdapter.__init__`

来源：[deploy/workforce/runtime-sdk/src/seven_agent/force/agent.py:273](../../../deploy/workforce/runtime-sdk/src/seven_agent/force/agent.py#L273)。同步方法。

```python
def AgentAdapter.__init__(self, *, name: str, version: str, capabilities=()) -> 未注解（以实现为准）
```

| 参数 | Python 类型 | 默认／调用要求 |
| --- | --- | --- |
| `name` | `str` | 必传；仅关键字参数 |
| `version` | `str` | 必传；仅关键字参数 |
| `capabilities` | `未注解` | `()`；仅关键字参数 |

#### `AgentAdapter.run`

来源：[deploy/workforce/runtime-sdk/src/seven_agent/force/agent.py:292](../../../deploy/workforce/runtime-sdk/src/seven_agent/force/agent.py#L292)。同步方法。

```python
def AgentAdapter.run(self, handler: Handler) -> 未注解（以实现为准）
```

| 参数 | Python 类型 | 默认／调用要求 |
| --- | --- | --- |
| `handler` | `Handler` | 必传 |

#### `AgentAdapter.resume`

来源：[deploy/workforce/runtime-sdk/src/seven_agent/force/agent.py:295](../../../deploy/workforce/runtime-sdk/src/seven_agent/force/agent.py#L295)。同步方法。

```python
def AgentAdapter.resume(self, handler: Handler) -> 未注解（以实现为准）
```

| 参数 | Python 类型 | 默认／调用要求 |
| --- | --- | --- |
| `handler` | `Handler` | 必传 |

#### `AgentAdapter.message`

来源：[deploy/workforce/runtime-sdk/src/seven_agent/force/agent.py:298](../../../deploy/workforce/runtime-sdk/src/seven_agent/force/agent.py#L298)。同步方法。

```python
def AgentAdapter.message(self, handler: Callable[[UserMessage, ExecutionContext], Awaitable[DeliveryStatus]]) -> 未注解（以实现为准）
```

| 参数 | Python 类型 | 默认／调用要求 |
| --- | --- | --- |
| `handler` | `Callable[[UserMessage, ExecutionContext], Awaitable[DeliveryStatus]]` | 必传 |

#### `AgentAdapter.suspend`

来源：[deploy/workforce/runtime-sdk/src/seven_agent/force/agent.py:301](../../../deploy/workforce/runtime-sdk/src/seven_agent/force/agent.py#L301)。同步方法。

```python
def AgentAdapter.suspend(self, handler: Callable[[RunRequest, ExecutionContext], Awaitable[str]]) -> 未注解（以实现为准）
```

| 参数 | Python 类型 | 默认／调用要求 |
| --- | --- | --- |
| `handler` | `Callable[[RunRequest, ExecutionContext], Awaitable[str]]` | 必传 |

Handler quiesces native execution and returns its real session reference.


#### `AgentAdapter.capabilities`

来源：[deploy/workforce/runtime-sdk/src/seven_agent/force/agent.py:306](../../../deploy/workforce/runtime-sdk/src/seven_agent/force/agent.py#L306)。同步方法。

```python
def AgentAdapter.capabilities(self) -> 未注解（以实现为准）
```

| 参数 | Python 类型 | 默认／调用要求 |
| --- | --- | --- |
这是 `@property` 属性，读取时不加括号。

#### `AgentAdapter.serve_streams`

来源：[deploy/workforce/runtime-sdk/src/seven_agent/force/agent.py:320](../../../deploy/workforce/runtime-sdk/src/seven_agent/force/agent.py#L320)。异步方法，需 `await`。

```python
async def AgentAdapter.serve_streams(self, reader, writer) -> 未注解（以实现为准）
```

| 参数 | Python 类型 | 默认／调用要求 |
| --- | --- | --- |
| `reader` | `未注解` | 必传 |
| `writer` | `未注解` | 必传 |

One initialized session. Caller owns stream creation and closure.


#### `AgentAdapter.serve`

来源：[deploy/workforce/runtime-sdk/src/seven_agent/force/agent.py:674](../../../deploy/workforce/runtime-sdk/src/seven_agent/force/agent.py#L674)。同步方法。

```python
def AgentAdapter.serve(self) -> 未注解（以实现为准）
```

| 参数 | Python 类型 | 默认／调用要求 |
| --- | --- | --- |
## 模块 `interaction`

### `DeliveryStatus`

`RECEIVED = 'RECEIVED'`
`REJECTED = 'REJECTED'`
`UNKNOWN = 'UNKNOWN'`

#### `key`

来源：[deploy/workforce/runtime-sdk/src/seven_agent/force/interaction.py:25](../../../deploy/workforce/runtime-sdk/src/seven_agent/force/interaction.py#L25)。同步方法。

```python
def key(value) -> 未注解（以实现为准）
```

| 参数 | Python 类型 | 默认／调用要求 |
| --- | --- | --- |
| `value` | `未注解` | 必传 |

#### `text`

来源：[deploy/workforce/runtime-sdk/src/seven_agent/force/interaction.py:30](../../../deploy/workforce/runtime-sdk/src/seven_agent/force/interaction.py#L30)。同步方法。

```python
def text(value, limit, *, nonempty=False) -> 未注解（以实现为准）
```

| 参数 | Python 类型 | 默认／调用要求 |
| --- | --- | --- |
| `value` | `未注解` | 必传 |
| `limit` | `未注解` | 必传 |
| `nonempty` | `未注解` | `False`；仅关键字参数 |

#### `question_key`

来源：[deploy/workforce/runtime-sdk/src/seven_agent/force/interaction.py:40](../../../deploy/workforce/runtime-sdk/src/seven_agent/force/interaction.py#L40)。同步方法。

```python
def question_key(value) -> 未注解（以实现为准）
```

| 参数 | Python 类型 | 默认／调用要求 |
| --- | --- | --- |
| `value` | `未注解` | 必传 |

#### `fields`

来源：[deploy/workforce/runtime-sdk/src/seven_agent/force/interaction.py:46](../../../deploy/workforce/runtime-sdk/src/seven_agent/force/interaction.py#L46)。同步方法。

```python
def fields(value, expected) -> 未注解（以实现为准）
```

| 参数 | Python 类型 | 默认／调用要求 |
| --- | --- | --- |
| `value` | `未注解` | 必传 |
| `expected` | `未注解` | 必传 |
### `QuestionOption`

| 字段 | Python 类型 | 默认值 |
| --- | --- | --- |
| `key` | `str` | 构造时提供 |
| `label` | `str` | 构造时提供 |

#### `QuestionOption.wire`

来源：[deploy/workforce/runtime-sdk/src/seven_agent/force/interaction.py:56](../../../deploy/workforce/runtime-sdk/src/seven_agent/force/interaction.py#L56)。同步方法。

```python
def QuestionOption.wire(self) -> 未注解（以实现为准）
```

| 参数 | Python 类型 | 默认／调用要求 |
| --- | --- | --- |
### `Question`

| 字段 | Python 类型 | 默认值 |
| --- | --- | --- |
| `key` | `str` | 构造时提供 |
| `title` | `str` | 构造时提供 |
| `description` | `str` | `''` |
| `kind` | `str` | `'TEXT'` |
| `required` | `bool` | `True` |
| `allow_free_text` | `bool` | `True` |
| `options` | `tuple[QuestionOption, ...]` | `()` |

#### `Question.wire`

来源：[deploy/workforce/runtime-sdk/src/seven_agent/force/interaction.py:72](../../../deploy/workforce/runtime-sdk/src/seven_agent/force/interaction.py#L72)。同步方法。

```python
def Question.wire(self) -> 未注解（以实现为准）
```

| 参数 | Python 类型 | 默认／调用要求 |
| --- | --- | --- |
### `QuestionRequest`

| 字段 | Python 类型 | 默认值 |
| --- | --- | --- |
| `interaction_key` | `str` | 构造时提供 |
| `questions` | `tuple[Question, ...]` | 构造时提供 |
| `is_blocking` | `bool` | `True` |

#### `QuestionRequest.from_wire`

来源：[deploy/workforce/runtime-sdk/src/seven_agent/force/interaction.py:101](../../../deploy/workforce/runtime-sdk/src/seven_agent/force/interaction.py#L101)。同步方法。

```python
def QuestionRequest.from_wire(cls, value) -> 未注解（以实现为准）
```

| 参数 | Python 类型 | 默认／调用要求 |
| --- | --- | --- |
| `value` | `未注解` | 必传 |

Parse the complete shared Question wire, including private restore input.


#### `QuestionRequest.wire`

来源：[deploy/workforce/runtime-sdk/src/seven_agent/force/interaction.py:110](../../../deploy/workforce/runtime-sdk/src/seven_agent/force/interaction.py#L110)。同步方法。

```python
def QuestionRequest.wire(self) -> 未注解（以实现为准）
```

| 参数 | Python 类型 | 默认／调用要求 |
| --- | --- | --- |
### `InteractionAnswer`

| 字段 | Python 类型 | 默认值 |
| --- | --- | --- |
| `question_key` | `str` | 构造时提供 |
| `selected_option_keys` | `tuple[str, ...]` | `()` |
| `text` | `str` | `''` |

#### `InteractionAnswer.wire`

来源：[deploy/workforce/runtime-sdk/src/seven_agent/force/interaction.py:131](../../../deploy/workforce/runtime-sdk/src/seven_agent/force/interaction.py#L131)。同步方法。

```python
def InteractionAnswer.wire(self) -> 未注解（以实现为准）
```

| 参数 | Python 类型 | 默认／调用要求 |
| --- | --- | --- |
### `UserMessage`

| 字段 | Python 类型 | 默认值 |
| --- | --- | --- |
| `round_key` | `str` | 构造时提供 |
| `message_key` | `str` | 构造时提供 |
| `text` | `str` | 构造时提供 |
### `InteractionResponse`

| 字段 | Python 类型 | 默认值 |
| --- | --- | --- |
| `interaction_key` | `str` | 构造时提供 |
| `response_id` | `str` | 构造时提供 |
| `answers` | `tuple[InteractionAnswer, ...]` | 构造时提供 |

#### `InteractionResponse.wire`

来源：[deploy/workforce/runtime-sdk/src/seven_agent/force/interaction.py:157](../../../deploy/workforce/runtime-sdk/src/seven_agent/force/interaction.py#L157)。同步方法。

```python
def InteractionResponse.wire(self) -> 未注解（以实现为准）
```

| 参数 | Python 类型 | 默认／调用要求 |
| --- | --- | --- |

#### `answers_from_wire`

来源：[deploy/workforce/runtime-sdk/src/seven_agent/force/interaction.py:170](../../../deploy/workforce/runtime-sdk/src/seven_agent/force/interaction.py#L170)。同步方法。

```python
def answers_from_wire(answers) -> 未注解（以实现为准）
```

| 参数 | Python 类型 | 默认／调用要求 |
| --- | --- | --- |
| `answers` | `未注解` | 必传 |

#### `validate_answers`

来源：[deploy/workforce/runtime-sdk/src/seven_agent/force/interaction.py:192](../../../deploy/workforce/runtime-sdk/src/seven_agent/force/interaction.py#L192)。同步方法。

```python
def validate_answers(request, answers) -> 未注解（以实现为准）
```

| 参数 | Python 类型 | 默认／调用要求 |
| --- | --- | --- |
| `request` | `未注解` | 必传 |
| `answers` | `未注解` | 必传 |

#### `validate_payload`

来源：[deploy/workforce/runtime-sdk/src/seven_agent/force/interaction.py:207](../../../deploy/workforce/runtime-sdk/src/seven_agent/force/interaction.py#L207)。同步方法。

```python
def validate_payload(method, payload) -> 未注解（以实现为准）
```

| 参数 | Python 类型 | 默认／调用要求 |
| --- | --- | --- |
| `method` | `未注解` | 必传 |
| `payload` | `未注解` | 必传 |
## 模块 `tools`

### `ToolDeliveryStatus`

`RECEIVED = 'RECEIVED'`
`REJECTED = 'REJECTED'`
`UNKNOWN = 'UNKNOWN'`
### `ToolTarget`

| 字段 | Python 类型 | 默认值 |
| --- | --- | --- |
| `host` | `str` | 构造时提供 |
| `protocol` | `str` | 构造时提供 |
| `port` | `int` | 构造时提供 |

#### `ToolTarget.from_wire`

来源：[deploy/workforce/runtime-sdk/src/seven_agent/force/tools.py:139](../../../deploy/workforce/runtime-sdk/src/seven_agent/force/tools.py#L139)。同步方法。

```python
def ToolTarget.from_wire(cls, value) -> 未注解（以实现为准）
```

| 参数 | Python 类型 | 默认／调用要求 |
| --- | --- | --- |
| `value` | `未注解` | 必传 |

#### `ToolTarget.wire`

来源：[deploy/workforce/runtime-sdk/src/seven_agent/force/tools.py:147](../../../deploy/workforce/runtime-sdk/src/seven_agent/force/tools.py#L147)。同步方法。

```python
def ToolTarget.wire(self) -> 未注解（以实现为准）
```

| 参数 | Python 类型 | 默认／调用要求 |
| --- | --- | --- |
### `ToolDefinition`

| 字段 | Python 类型 | 默认值 |
| --- | --- | --- |
| `name` | `str` | 构造时提供 |
| `description` | `str` | 构造时提供 |
| `input_schema` | `dict` | 构造时提供 |

#### `ToolDefinition.wire`

来源：[deploy/workforce/runtime-sdk/src/seven_agent/force/tools.py:168](../../../deploy/workforce/runtime-sdk/src/seven_agent/force/tools.py#L168)。同步方法。

```python
def ToolDefinition.wire(self) -> 未注解（以实现为准）
```

| 参数 | Python 类型 | 默认／调用要求 |
| --- | --- | --- |
### `ToolCallRequest`

| 字段 | Python 类型 | 默认值 |
| --- | --- | --- |
| `native_tool_call_id` | `str` | 构造时提供 |
| `local_tool_call_key` | `str` | 构造时提供 |
| `tool_name` | `str` | 构造时提供 |
| `arguments` | `dict` | 构造时提供 |

#### `ToolCallRequest.from_wire`

来源：[deploy/workforce/runtime-sdk/src/seven_agent/force/tools.py:189](../../../deploy/workforce/runtime-sdk/src/seven_agent/force/tools.py#L189)。同步方法。

```python
def ToolCallRequest.from_wire(cls, value) -> 未注解（以实现为准）
```

| 参数 | Python 类型 | 默认／调用要求 |
| --- | --- | --- |
| `value` | `未注解` | 必传 |

#### `ToolCallRequest.wire`

来源：[deploy/workforce/runtime-sdk/src/seven_agent/force/tools.py:198](../../../deploy/workforce/runtime-sdk/src/seven_agent/force/tools.py#L198)。同步方法。

```python
def ToolCallRequest.wire(self) -> 未注解（以实现为准）
```

| 参数 | Python 类型 | 默认／调用要求 |
| --- | --- | --- |
### `ToolResult`

| 字段 | Python 类型 | 默认值 |
| --- | --- | --- |
| `native_tool_call_id` | `str` | 构造时提供 |
| `local_tool_call_key` | `str` | 构造时提供 |
| `operation_id` | `str` | 构造时提供 |
| `status` | `str` | 构造时提供 |
| `exit_code` | `int \| None` | `None` |
| `summary` | `str` | `''` |
| `output` | `str` | `''` |
| `result_digest` | `str` | `''` |
| `reason_code` | `str` | `''` |

#### `ToolResult.from_wire`

来源：[deploy/workforce/runtime-sdk/src/seven_agent/force/tools.py:222](../../../deploy/workforce/runtime-sdk/src/seven_agent/force/tools.py#L222)。同步方法。

```python
def ToolResult.from_wire(cls, value) -> 未注解（以实现为准）
```

| 参数 | Python 类型 | 默认／调用要求 |
| --- | --- | --- |
| `value` | `未注解` | 必传 |

#### `ToolResult.wire`

来源：[deploy/workforce/runtime-sdk/src/seven_agent/force/tools.py:231](../../../deploy/workforce/runtime-sdk/src/seven_agent/force/tools.py#L231)。同步方法。

```python
def ToolResult.wire(self) -> 未注解（以实现为准）
```

| 参数 | Python 类型 | 默认／调用要求 |
| --- | --- | --- |

#### `validate_payload`

来源：[deploy/workforce/runtime-sdk/src/seven_agent/force/tools.py:265](../../../deploy/workforce/runtime-sdk/src/seven_agent/force/tools.py#L265)。同步方法。

```python
def validate_payload(method, payload) -> 未注解（以实现为准）
```

| 参数 | Python 类型 | 默认／调用要求 |
| --- | --- | --- |
| `method` | `未注解` | 必传 |
| `payload` | `未注解` | 必传 |
## 模块 `processor`


#### `pass_result`

来源：[deploy/workforce/runtime-sdk/src/seven_agent/force/processor/__init__.py:22](../../../deploy/workforce/runtime-sdk/src/seven_agent/force/processor/__init__.py#L22)。同步方法。

```python
def pass_result(*, reason_code: str='OK', message: str='', checks=None, findings=None, outputs=None, actions=None) -> dict[str, Any]
```

| 参数 | Python 类型 | 默认／调用要求 |
| --- | --- | --- |
| `reason_code` | `str` | `'OK'`；仅关键字参数 |
| `message` | `str` | `''`；仅关键字参数 |
| `checks` | `未注解` | `None`；仅关键字参数 |
| `findings` | `未注解` | `None`；仅关键字参数 |
| `outputs` | `未注解` | `None`；仅关键字参数 |
| `actions` | `未注解` | `None`；仅关键字参数 |

#### `ask_agent`

来源：[deploy/workforce/runtime-sdk/src/seven_agent/force/processor/__init__.py:26](../../../deploy/workforce/runtime-sdk/src/seven_agent/force/processor/__init__.py#L26)。同步方法。

```python
def ask_agent(*, reason_code: str, message: str, checks=None, findings=None) -> dict[str, Any]
```

| 参数 | Python 类型 | 默认／调用要求 |
| --- | --- | --- |
| `reason_code` | `str` | 必传；仅关键字参数 |
| `message` | `str` | 必传；仅关键字参数 |
| `checks` | `未注解` | `None`；仅关键字参数 |
| `findings` | `未注解` | `None`；仅关键字参数 |

#### `wait_user`

来源：[deploy/workforce/runtime-sdk/src/seven_agent/force/processor/__init__.py:30](../../../deploy/workforce/runtime-sdk/src/seven_agent/force/processor/__init__.py#L30)。同步方法。

```python
def wait_user(*, reason_code: str, message: str, questions=None, findings=None, outputs=None) -> dict[str, Any]
```

| 参数 | Python 类型 | 默认／调用要求 |
| --- | --- | --- |
| `reason_code` | `str` | 必传；仅关键字参数 |
| `message` | `str` | 必传；仅关键字参数 |
| `questions` | `未注解` | `None`；仅关键字参数 |
| `findings` | `未注解` | `None`；仅关键字参数 |
| `outputs` | `未注解` | `None`；仅关键字参数 |

#### `reject`

来源：[deploy/workforce/runtime-sdk/src/seven_agent/force/processor/__init__.py:34](../../../deploy/workforce/runtime-sdk/src/seven_agent/force/processor/__init__.py#L34)。同步方法。

```python
def reject(*, reason_code: str, message: str, checks=None, findings=None) -> dict[str, Any]
```

| 参数 | Python 类型 | 默认／调用要求 |
| --- | --- | --- |
| `reason_code` | `str` | 必传；仅关键字参数 |
| `message` | `str` | 必传；仅关键字参数 |
| `checks` | `未注解` | `None`；仅关键字参数 |
| `findings` | `未注解` | `None`；仅关键字参数 |

#### `manual_review`

来源：[deploy/workforce/runtime-sdk/src/seven_agent/force/processor/__init__.py:38](../../../deploy/workforce/runtime-sdk/src/seven_agent/force/processor/__init__.py#L38)。同步方法。

```python
def manual_review(*, reason_code: str, message: str, checks=None, findings=None, questions=None, outputs=None) -> dict[str, Any]
```

| 参数 | Python 类型 | 默认／调用要求 |
| --- | --- | --- |
| `reason_code` | `str` | 必传；仅关键字参数 |
| `message` | `str` | 必传；仅关键字参数 |
| `checks` | `未注解` | `None`；仅关键字参数 |
| `findings` | `未注解` | `None`；仅关键字参数 |
| `questions` | `未注解` | `None`；仅关键字参数 |
| `outputs` | `未注解` | `None`；仅关键字参数 |

#### `workspace_path`

来源：[deploy/workforce/runtime-sdk/src/seven_agent/force/processor/__init__.py:42](../../../deploy/workforce/runtime-sdk/src/seven_agent/force/processor/__init__.py#L42)。同步方法。

```python
def workspace_path(*, name: str, path: str, storage: str='IMMEDIATE') -> dict[str, str]
```

| 参数 | Python 类型 | 默认／调用要求 |
| --- | --- | --- |
| `name` | `str` | 必传；仅关键字参数 |
| `path` | `str` | 必传；仅关键字参数 |
| `storage` | `str` | `'IMMEDIATE'`；仅关键字参数 |

#### `archive_output`

来源：[deploy/workforce/runtime-sdk/src/seven_agent/force/processor/__init__.py:46](../../../deploy/workforce/runtime-sdk/src/seven_agent/force/processor/__init__.py#L46)。同步方法。

```python
def archive_output(*, name: str, files: list[str], excludes: list[str] | None=None, format: str='ZIP', storage: str='ON_DEMAND') -> dict[str, Any]
```

| 参数 | Python 类型 | 默认／调用要求 |
| --- | --- | --- |
| `name` | `str` | 必传；仅关键字参数 |
| `files` | `list[str]` | 必传；仅关键字参数 |
| `excludes` | `list[str] \| None` | `None`；仅关键字参数 |
| `format` | `str` | `'ZIP'`；仅关键字参数 |
| `storage` | `str` | `'ON_DEMAND'`；仅关键字参数 |

Declare a platform-built archive from the frozen trusted result.

``files`` and ``excludes`` are selection patterns, not a self-reported
manifest. The Result Coordinator resolves the concrete files and the
post-process Worker creates deterministic ZIP/TAR.GZ bytes later.


#### `get_env`

来源：[deploy/workforce/runtime-sdk/src/seven_agent/force/processor/__init__.py:70](../../../deploy/workforce/runtime-sdk/src/seven_agent/force/processor/__init__.py#L70)。同步方法。

```python
def get_env(name: str, default_value: str | None=None) -> str | None
```

| 参数 | Python 类型 | 默认／调用要求 |
| --- | --- | --- |
| `name` | `str` | 必传 |
| `default_value` | `str \| None` | `None` |

Return a cycle-scoped CONFIG/SECRET value approved by the platform.

Values are present only while ``process(context)`` runs.  The helper is a
convenience for authoring templates; it does not widen the Processor's
authority beyond the environment bindings frozen in the Task snapshot.


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
