"""Fixed CORRECTION_CYCLE Reviewer image entrypoint."""
from adapter import build_adapter

adapter = build_adapter("review.json", "37d95a77145cc5fc8e0246abd1f2159bfa778a5e4db2cf1c0fd90b01eb6f60e3")

if __name__ == "__main__":
    adapter.serve()
