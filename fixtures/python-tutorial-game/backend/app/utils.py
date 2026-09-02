import re

BANNED_WORDS = ['eval', 'exec', 'os', 'sys', 'exec', 
'python', 'read', 'write', 'utils', 'dir', '__', 'quit', 'exit', 'globals', 'locals',
'urllib', 'request', 'logging', 'print', 'multiprocessing', 'raise', 'throw', 'types',
'shutil',
'pathlib',
'os.path',
'fileinput',
'stat',
'filecmp',
'tempfile',
'glob',
'fnmatch',
'linecache',
'shutil']

def validate(code: str):
    if any(banned_word in code for banned_word in BANNED_WORDS):
        return {'ok': False, 'reason': "Код содержит недопустимое выражение: <данные удалены>"}
    return {'ok': True, 'reason': "Всё ок"}
