#!/usr/bin/env python3
"""Wails/Zig Darwin compiler wrapper, preserving argument boundaries and 12.0 floor."""
import os
import sys


def arguments(args, arch):
    target = {'arm64': 'aarch64-macos.12.0', 'amd64': 'x86_64-macos.12.0'}[arch]
    result = ['zig', 'c++' if any(a.endswith(('.cc', '.cpp', '.mm')) for a in args) else 'cc',
              '-target', target, '-fno-sanitize=all', '-isysroot', '/opt/macos-sdk',
              '-I/opt/macos-sdk/usr/include', '-L/opt/macos-sdk/usr/lib',
              '-F/opt/macos-sdk/System/Library/Frameworks']
    skip = False
    for a in args:
        if skip:
            skip = False
        elif a in ('-target', '--target'):
            skip = True  # Go's host target must not override the deployment floor.
        elif a.startswith(('-mmacosx-version-min=', '--target=')):
            continue
        else:
            result.append(a)
    return result


if __name__ == '__main__':
    command = arguments(sys.argv[1:], os.environ['GOARCH'])
    os.execvp(command[0], command)
