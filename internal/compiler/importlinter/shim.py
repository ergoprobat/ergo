"""Run import-linter on a copy of a project without importing the project.

ipso runs this file with python -I -c. argv: config, import-linter version, grimp version,
the directory that holds the copied root packages, then the root package names.

import-linter inserts the working directory in sys.path and then resolves each root package by
name, and grimp resolves it with importlib.util.find_spec. A finder first on sys.meta_path answers
the root names with the copy and a loader that refuses to execute anything, so repository code is
parsed by grimp and never imported.
"""

import importlib.abc
import importlib.machinery
import os
import sys

STATUS_VERSION = 3
STATUS_ROOT = 4


class RefusingLoader(importlib.abc.Loader):
    def create_module(self, spec):
        raise ImportError("ipso does not import repository code: " + spec.name)

    def exec_module(self, module):
        raise ImportError("ipso does not import repository code: " + module.__name__)


class RootFinder(importlib.abc.MetaPathFinder):
    def __init__(self, source, roots):
        self._directories = {name: os.path.join(source, name) for name in roots}

    def find_spec(self, name, path=None, target=None):
        directory = self._directories.get(name)
        if directory is None or path is not None:
            return None
        spec = importlib.machinery.ModuleSpec(
            name,
            RefusingLoader(),
            origin=os.path.join(directory, "__init__.py"),
            is_package=True,
        )
        spec.submodule_search_locations = [directory]
        spec.has_location = True
        return spec


def main(argv):
    config, linter_version, grimp_version, source = argv[:4]
    roots = argv[4:]

    import grimp
    import importlinter
    from importlinter import cli
    from importlinter.application import output

    if importlinter.__version__ != linter_version or grimp.__version__ != grimp_version:
        return STATUS_VERSION
    # A module that is already imported is found in sys.modules before any finder runs, so a root
    # with that name would analyse the installed module instead of the copy.
    for name in roots:
        if name in sys.modules:
            sys.stderr.write("root package " + name + " is a module the engine uses\n")
            return STATUS_ROOT

    sys.meta_path.insert(0, RootFinder(source, roots))
    # Rich turns :name: into an emoji, which would change a contract name in the report.
    output.console._emoji = False
    return cli.lint_imports(config_filename=config, no_cache=True, no_logo=True)


sys.exit(main(sys.argv[1:]))
