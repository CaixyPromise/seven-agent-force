"""Typed, bounded local interaction contracts. None of these grant authority."""
from __future__ import annotations

import dataclasses
import enum
import re

from ._protocol import ProtocolError, VERSION, decode, encode

MAX_ANSWERS_BYTES = 16 << 10
MAX_QUESTIONS = 3
MAX_OPTIONS = 8
INTERACTION_METHODS = frozenset({
    "interaction.request", "interaction.respond", "interaction.received",
    "interaction.closed", "user.message", "message.received", "execution.suspend", "execution.suspended",
})


class DeliveryStatus(str, enum.Enum):
    RECEIVED = "RECEIVED"
    REJECTED = "REJECTED"
    UNKNOWN = "UNKNOWN"


def key(value):
    if not isinstance(value, str) or re.fullmatch(r"[A-Za-z0-9._:-]{1,128}", value) is None:
        raise ProtocolError()


def text(value, limit, *, nonempty=False):
    if not isinstance(value, str) or (nonempty and not value.strip()) or "\0" in value:
        raise ProtocolError()
    try:
        if len(value.encode("utf-8")) > limit:
            raise ProtocolError()
    except UnicodeError:
        raise ProtocolError() from None


def question_key(value):
    text(value, 128, nonempty=True)
    if value.strip() != value:
        raise ProtocolError()


def fields(value, expected):
    if not isinstance(value, dict) or set(value) != set(expected.split()):
        raise ProtocolError()


@dataclasses.dataclass(frozen=True)
class QuestionOption:
    key: str
    label: str

    def wire(self):
        question_key(self.key)
        text(self.label, 512, nonempty=True)
        return {"key": self.key, "label": self.label}


@dataclasses.dataclass(frozen=True)
class Question:
    key: str
    title: str
    description: str = ""
    kind: str = "TEXT"
    required: bool = True
    allow_free_text: bool = True
    options: tuple[QuestionOption, ...] = ()

    def wire(self):
        question_key(self.key)
        text(self.title, 512, nonempty=True)
        text(self.description, 4096)
        if (self.kind not in ("SINGLE_CHOICE", "MULTIPLE_CHOICE", "TEXT")
                or type(self.required) is not bool or type(self.allow_free_text) is not bool
                or not isinstance(self.options, (tuple, list))):
            raise ProtocolError()
        if (len(self.options) > MAX_OPTIONS
                or (self.kind == "TEXT" and self.options)
                or (self.kind != "TEXT" and not self.options)):
            raise ProtocolError()
        if any(not isinstance(option, QuestionOption) for option in self.options):
            raise ProtocolError()
        options = [option.wire() for option in self.options]
        if len({option["key"] for option in options}) != len(options):
            raise ProtocolError()
        return {"key": self.key, "title": self.title, "description": self.description,
                "kind": self.kind, "required": self.required, "options": options,
                "allowFreeText": self.allow_free_text}


@dataclasses.dataclass(frozen=True)
class QuestionRequest:
    interaction_key: str
    questions: tuple[Question, ...]
    is_blocking: bool = True

    @classmethod
    def from_wire(cls, value):
        """Parse the complete shared Question wire, including private restore input."""
        fields(value, "interactionKey questions isBlocking")
        validate_payload("interaction.request", {"roundKey": "restore", **value})
        return cls(value["interactionKey"], tuple(
            Question(q["key"], q["title"], q["description"], q["kind"], q["required"],
                     q["allowFreeText"], tuple(QuestionOption(**o) for o in q["options"]))
            for q in value["questions"]), value["isBlocking"])

    def wire(self):
        key(self.interaction_key)
        if not isinstance(self.questions, (tuple, list)) or not 1 <= len(self.questions) <= MAX_QUESTIONS:
            raise ProtocolError()
        if any(not isinstance(question, Question) for question in self.questions):
            raise ProtocolError()
        questions = [question.wire() for question in self.questions]
        if len({question["key"] for question in questions}) != len(questions):
            raise ProtocolError()
        if type(self.is_blocking) is not bool:
            raise ProtocolError()
        return {"interactionKey": self.interaction_key, "questions": questions,
                "isBlocking": self.is_blocking}


@dataclasses.dataclass(frozen=True)
class InteractionAnswer:
    question_key: str
    selected_option_keys: tuple[str, ...] = ()
    text: str = ""

    def wire(self):
        question_key(self.question_key)
        text(self.text, MAX_ANSWERS_BYTES)
        if not isinstance(self.selected_option_keys, (tuple, list)) or len(self.selected_option_keys) > MAX_OPTIONS:
            raise ProtocolError()
        for option in self.selected_option_keys:
            question_key(option)
        if len(set(self.selected_option_keys)) != len(self.selected_option_keys):
            raise ProtocolError()
        return {"questionKey": self.question_key, "selectedOptionKeys": list(self.selected_option_keys), "text": self.text}


@dataclasses.dataclass(frozen=True)
class UserMessage:
    round_key: str
    message_key: str
    text: str


@dataclasses.dataclass(frozen=True)
class InteractionResponse:
    """Typed response identity for accepted snapshots or expected cold delivery."""
    interaction_key: str
    response_id: str
    answers: tuple[InteractionAnswer, ...]

    def wire(self):
        if not isinstance(self.answers, (tuple, list)) or any(
                not isinstance(answer, InteractionAnswer) for answer in self.answers):
            raise ProtocolError()
        value = {"interactionKey": self.interaction_key, "responseId": self.response_id,
                 "answers": [answer.wire() for answer in self.answers]}
        # Use the same codec (including decimal int64 IDs and bounded answers)
        # as real downstream delivery, never a reduced private-input validator.
        decode(encode({"protocol": VERSION, "method": "interaction.respond", "key": "validate",
                       "payload": {"roundKey": "validate", **value}}), direction="down")
        return value


def answers_from_wire(answers):
    if not isinstance(answers, list) or len(answers) > MAX_QUESTIONS:
        raise ProtocolError()
    # Match backend Go json.Marshal byte accounting, including HTML escaping.
    canonical = encode(answers)[:-1]
    for character in "<>&\u2028\u2029":
        canonical = canonical.replace(character.encode(), ("\\u%04x" % ord(character)).encode())
    if len(canonical) > MAX_ANSWERS_BYTES:
        raise ProtocolError("ADAPTER_ANSWERS_TOO_LARGE")
    result = []
    for answer in answers:
        fields(answer, "questionKey selectedOptionKeys text")
        if not isinstance(answer["selectedOptionKeys"], list):
            raise ProtocolError()
        item = InteractionAnswer(answer["questionKey"], tuple(answer["selectedOptionKeys"]), answer["text"])
        item.wire()
        result.append(item)
    if len({item.question_key for item in result}) != len(result):
        raise ProtocolError()
    return tuple(result)


def validate_answers(request, answers):
    questions = {question["key"]: question for question in request["questions"]}
    answered = {answer.question_key for answer in answers}
    if answered - set(questions) or any(question["required"] and name not in answered for name, question in questions.items()):
        raise ProtocolError("ADAPTER_ANSWER_INVALID")
    for answer in answers:
        question = questions[answer.question_key]
        if (set(answer.selected_option_keys) - {option["key"] for option in question["options"]}
                or (answer.text and question["kind"] != "TEXT" and not question["allowFreeText"])
                or (question["kind"] == "SINGLE_CHOICE" and len(answer.selected_option_keys) > 1)
                or (question["kind"] == "TEXT" and answer.selected_option_keys)
                or (question["required"] and not (answer.selected_option_keys or answer.text.strip()))):
            raise ProtocolError("ADAPTER_ANSWER_INVALID")


def validate_payload(method, payload):
    if method not in INTERACTION_METHODS:
        return
    expected = {
        "interaction.request": "roundKey interactionKey questions isBlocking",
        "interaction.respond": "roundKey interactionKey responseId answers",
        "interaction.received": "roundKey interactionKey responseId status",
        "interaction.closed": "roundKey interactionKey reason",
        "user.message": "roundKey messageKey text",
        "message.received": "roundKey messageKey status",
        "execution.suspend": "roundKey",
        "execution.suspended": "roundKey sessionReference",
    }
    fields(payload, expected[method])
    key(payload["roundKey"])
    for name in ("interactionKey", "messageKey"):
        if name in payload:
            key(payload[name])
    if method == "interaction.request":
        if type(payload["isBlocking"]) is not bool:
            raise ProtocolError()
        questions = payload["questions"]
        if not isinstance(questions, list) or not 1 <= len(questions) <= MAX_QUESTIONS:
            raise ProtocolError()
        typed = []
        for question in questions:
            fields(question, "key title description kind required options allowFreeText")
            if not isinstance(question["options"], list) or len(question["options"]) > MAX_OPTIONS:
                raise ProtocolError()
            for option in question["options"]:
                fields(option, "key label")
            typed.append(Question(question["key"], question["title"], question["description"],
                                  question["kind"], question["required"], question["allowFreeText"],
                                  tuple(QuestionOption(**option) for option in question["options"])))
        QuestionRequest(payload["interactionKey"], tuple(typed), payload["isBlocking"]).wire()
    elif method == "interaction.respond":
        answers_from_wire(payload["answers"])
    elif method == "user.message":
        text(payload["text"], 16 << 10, nonempty=True)
    elif method.endswith(".received"):
        if payload["status"] not in {status.value for status in DeliveryStatus}:
            raise ProtocolError()
    elif method == "interaction.closed":
        if payload["reason"] not in {"NATIVE_REQUEST_CLEARED", "TURN_COMPLETED", "TURN_INTERRUPTED"}:
            raise ProtocolError()
    elif method == "execution.suspended":
        text(payload["sessionReference"], 4096, nonempty=True)
