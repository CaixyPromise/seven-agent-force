"""Fixed Batch3 answer/supplement/suspend fixture; no runtime scenario choice."""
import sys

sys.path[:0] = ["/opt/seven-worker/result-runtime", "/opt/seven/adapter"]

from interaction import build_adapter

adapter = build_adapter("673817ce03666466c87b50df39b92c0e12176164380a95797543c73901b7becb")

if __name__ == "__main__":
    adapter.serve()
