#!/usr/bin/env python3
"""Wrapper to run image_duplicate_test with proper numpy handling"""
import sys
import json
import numpy as np

# We'll monkey-patch json to handle numpy types
class NumpyEncoder(json.JSONEncoder):
    def default(self, obj):
        if isinstance(obj, (np.integer,)):
            return int(obj)
        if isinstance(obj, (np.floating,)):
            return float(obj)
        if isinstance(obj, (np.bool_,)):
            return bool(obj)
        if isinstance(obj, np.ndarray):
            return obj.tolist()
        if obj is None:
            return None
        return super().default(obj)

# Save original dumps
original_dumps = json.dumps

def patched_dumps(obj, **kwargs):
    kwargs.setdefault('cls', NumpyEncoder)
    return original_dumps(obj, **kwargs)

json.dumps = patched_dumps

# Now import and run the original script
sys.argv = ['image_duplicate_test.py', 
    '--input_dir', '/home/program/qq-workspace/self-workplace/geng-skills/paper_images/figures',
    '--threshold', '0.85',
    '--output', '/home/program/qq-workspace/self-workplace/geng-skills/paper_images/duplicate_results.json']

exec(open('scripts/image_duplicate_test.py').read())
