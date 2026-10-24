"""Compatibility imports for the Seven Agent Force runtime SDK."""
import importlib
import sys

for _name in ('_protocol', 'interaction', 'tools', 'processor', 'agent', '_project'):
    _module = importlib.import_module('seven_agent.force.' + _name)
    globals()[_name] = _module
    sys.modules[__name__ + '.' + _name] = _module
