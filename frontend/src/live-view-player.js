// Video-only MSE adapter. Camera URLs and credentials never enter this module.
export class CameraPlayer {
  constructor(video, image, onFrame, onError, Media = globalThis.MediaSource) {
    Object.assign(this, { video, image, onFrame, onError, Media });
    this.queue = [];
    this.bytes = 0;
    this.closed = false;
    this.info = null;
    this.video.onerror = () =>
      this.fail(
        "The system decoder could not play this video. Try an H.264 sub stream; smart-codec and H.265 support varies.",
      );
    const frame = () => {
      if (this.closed) return;
      this.onFrame();
      this.frameID = video.requestVideoFrameCallback?.(frame);
    };
    if (video.requestVideoFrameCallback)
      this.frameID = video.requestVideoFrameCallback(frame);
    else
      video.ontimeupdate = () => {
        if (!video.paused && video.currentTime !== this.lastTime) {
          this.lastTime = video.currentTime;
          onFrame();
        }
      };
  }
  information(info) {
    if (this.closed) return;
    if (this.info?.mime === info.mime) return;
    if (this.info)
      return this.fail(
        "Camera encoding changed during playback. Stop and reopen the preview.",
      );
    this.info = info;
    const jpeg = info.mime === "image/jpeg";
    this.video.hidden = jpeg;
    this.image.hidden = !jpeg;
    if (jpeg) return;
    if (!this.Media || !this.Media.isTypeSupported(info.mime))
      return this.fail(
        `${info.codec || "This codec"} is not supported by this system's video decoder. Select an H.264 sub stream or an HTTP JPEG endpoint.`,
      );
    this.media = new this.Media();
    this.url = URL.createObjectURL(this.media);
    this.video.src = this.url;
    this.media.addEventListener(
      "sourceopen",
      () => {
        if (this.closed) return;
        try {
          this.buffer = this.media.addSourceBuffer(info.mime);
          this.buffer.addEventListener("updateend", () => this.flush());
          this.buffer.addEventListener("error", () =>
            this.fail(
              "Camera video could not be decoded. Try another H.264 profile.",
            ),
          );
          this.flush();
          this.video
            .play()
            .catch(() =>
              this.fail(
                "Playback was blocked by the system video component. Close this panel and open it again.",
              ),
            );
        } catch {
          this.fail(
            "The system could not initialise this video decoder. Try an H.264 sub stream.",
          );
        }
      },
      { once: true },
    );
  }
  packet(kind, bytes) {
    if (this.closed) return;
    if (kind === 2) {
      if (bytes.length > 4 * 1024 * 1024 || this.info?.mime !== "image/jpeg")
        return this.fail("Invalid camera image.");
      const previous = this.imageURL;
      this.imageURL = URL.createObjectURL(
        new Blob([bytes], { type: "image/jpeg" }),
      );
      this.image.onload = () => {
        if (!this.closed) this.onFrame();
      };
      this.image.onerror = () =>
        this.fail("Camera returned an invalid JPEG image.");
      this.image.src = this.imageURL;
      if (previous) URL.revokeObjectURL(previous);
      return;
    }
    if (kind !== 1 || !this.info || this.info.mime === "image/jpeg")
      return this.fail("Unexpected camera media packet.");
    if (this.bytes + bytes.length > 8 * 1024 * 1024 || this.queue.length >= 160)
      return this.fail(
        "The player cannot keep up. Select the sub stream or a lower resolution.",
      );
    this.queue.push(bytes);
    this.bytes += bytes.length;
    this.flush();
  }
  flush() {
    if (this.closed || !this.buffer || this.buffer.updating) return;
    try {
      const ranges = this.buffer.buffered;
      if (ranges.length) {
        const end = ranges.end(ranges.length - 1);
        if (end - this.video.currentTime > 2.5)
          this.video.currentTime = Math.max(0, end - 0.25);
        if (this.video.currentTime - ranges.start(0) > 6) {
          this.buffer.remove(0, this.video.currentTime - 4);
          return;
        }
      }
      if (this.queue.length) {
        const data = this.queue.shift();
        this.bytes -= data.length;
        this.buffer.appendBuffer(data);
      }
    } catch {
      this.fail("Video buffering failed. Select the sub stream or Retry.");
    }
  }
  fail(message) {
    if (this.closed) return;
    this.close();
    this.onError(message);
  }
  close() {
    if (this.closed) return;
    this.closed = true;
    this.queue = [];
    this.bytes = 0;
    this.video.cancelVideoFrameCallback?.(this.frameID);
    this.video.onerror = null;
    this.video.ontimeupdate = null;
    this.video.pause();
    this.video.removeAttribute("src");
    this.video.load();
    this.image.onload = null;
    this.image.onerror = null;
    this.image.removeAttribute("src");
    if (this.url) URL.revokeObjectURL(this.url);
    if (this.imageURL) URL.revokeObjectURL(this.imageURL);
    this.buffer = null;
    this.media = null;
  }
}
