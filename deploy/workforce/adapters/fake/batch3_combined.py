"""Fixed Batch3 AGENT/REVIEW entries; role selects protocol, never verdict."""
import sys

sys.path[:0] = ["/opt/seven-worker/result-runtime", "/opt/seven/adapter"]

from interaction import build_adapter
from batch3_review import run as review_run

adapter = build_adapter("673817ce03666466c87b50df39b92c0e12176164380a95797543c73901b7becb", review_handler=review_run)


if __name__ == "__main__":
    adapter.serve()
