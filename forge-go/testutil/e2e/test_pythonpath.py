import sys
import os
print("PYTHONPATH:", os.getenv("PYTHONPATH"))
print("sys.path:")
for p in sys.path:
    print("  ", p)
try:
    import rustic_ai.core
    print("SUCCESS importing rustic_ai.core!")
except Exception as e:
    print("FAILED importing rustic_ai.core:", e)
