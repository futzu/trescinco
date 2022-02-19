#!/usr/bin/env python3

"""
re_cc.py clears and sets 
continuity counters in mpegts files.

use like:

        python3 re_cc.py video.ts

        
"""

import os
import sys
from threefive import Stream


if __name__ == "__main__":
    args = sys.argv[1:]
    for arg in args:
        strm = Stream(arg)
        strm.re_cc()
        os.rename("re_cc.tmp",arg)
