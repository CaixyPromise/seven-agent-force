import unittest
import json
from pathlib import Path

from seven_agent.force._protocol import ProtocolError, decode, encode


class ProtocolTests(unittest.TestCase):
    def test_shared_frames(self):
        path = Path(__file__).resolve().parents[1] / "protocol" / "frames.json"
        for fixture in json.loads(path.read_text()):
            with self.subTest(name=fixture["name"]):
                try:
                    decode(fixture["wire"].encode(), direction=fixture["direction"])
                    valid = True
                except ProtocolError:
                    valid = False
                self.assertEqual(valid, fixture["valid"])

    def frame(self, **changes):
        value = dict(protocol="seven.agent-adapter/v1", method="initialize",
                     key="init-1", payload={})
        value.update(changes)
        return value

    def test_round_trip(self):
        value = self.frame()
        self.assertEqual(decode(encode(value), direction="down"), value)

    def test_unknown_method_and_wrong_direction(self):
        for value in (self.frame(method="approve"), self.frame(method="ready")):
            with self.assertRaises(ProtocolError):
                decode(encode(value), direction="down")

    def test_duplicate_keys_nonfinite_and_nonobject(self):
        for raw in (b'{"key":"a","key":"b"}\n', b'{"payload":NaN}\n', b'[]\n'):
            with self.assertRaises(ProtocolError):
                decode(raw, direction="down")

    def test_frame_limit_and_newline(self):
        for raw in (b'x' * ((1 << 20) + 1), b'{}', b'{}\n{}\n'):
            with self.assertRaises(ProtocolError):
                decode(raw, direction="down")

    def test_entity_ids_are_positive_decimal_strings(self):
        for value in (1, "01", "+1", "1e2", "0", "9223372036854775808"):
            with self.assertRaises(ProtocolError):
                decode(encode(self.frame(payload={"taskId": value})), direction="down")

    def test_message_limit_utf8_bytes(self):
        frame = self.frame(method="message", payload={"text": "中" * 22000})
        with self.assertRaises(ProtocolError):
            decode(encode(frame), direction="up")


if __name__ == "__main__":
    unittest.main()
