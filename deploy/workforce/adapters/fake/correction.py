"""Fixed CORRECTION_CYCLE Agent image entrypoint."""
from adapter import build_adapter

adapter = build_adapter("correction.json", "5c30b1ad526103d282fd06f25cb4ec15787ee5707c969fa993821e7abee37e5c")

if __name__ == "__main__":
    adapter.serve()
