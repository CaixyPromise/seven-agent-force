"""Fixed BATCH1_COMPLETION image entrypoint."""
from adapter import build_adapter

adapter = build_adapter("batch1.json", "0174ee3ce514c1bf06daa0dc4ac88ea372913ea8f707e3c5764b57fd9f04b7af")

if __name__ == "__main__":
    adapter.serve()
