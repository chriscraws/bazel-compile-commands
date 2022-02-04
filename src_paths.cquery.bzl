# A formatting function for Bazel cquery results
#
# Formats the target as the path of its first file, if that file has a C++ extension..

_extensions = [
    "cc",
    "cpp",
    "cxx",
    "h",
    "hh",
    "hpp",
    "hxx",
    "ipp",
]

def format(target):
    files = target.files.to_list()
    if len(files) == 0:
        return ""
    f = files[0]
    if f.extension not in _extensions:
        return ""
    return f.path
