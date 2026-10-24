# Synthetic video fixtures

These 320 × 180, 10 fps, one-second videos contain FFmpeg's generated `testsrc2`
pattern, not a physical camera capture. They contain no audio or real credentials.
The fixture server loops them over a loopback-only RTSP socket. It is imported
only by tests and the explicitly tagged synthetic camera development command.

Generated on Sparx with FFmpeg 8.0.1:

```sh
ffmpeg -f lavfi -i testsrc2=size=320x180:rate=10 -t 1 -an -c:v libx264 -profile:v baseline -pix_fmt yuv420p -preset ultrafast -tune zerolatency -g 10 -x264-params aud=1 -f h264 synthetic.h264
ffmpeg -f lavfi -i testsrc2=size=320x180:rate=10 -t 1 -an -c:v libx265 -pix_fmt yuv420p -preset ultrafast -x265-params pools=none:frame-threads=1:aud=1:keyint=10:min-keyint=10:bframes=0:log-level=error -f hevc synthetic.h265
```

FFmpeg is used to generate test inputs only; it is not an application dependency
and is not redistributed. These generated fixtures are provided under the
project's MIT licence.
