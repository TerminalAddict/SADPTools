// Variance of a four-neighbour Laplacian on downsampled luminance. A relative
// heuristic only: scene texture, lighting, motion and compression all affect it.
export function sharpness(data, width, height) {
  if (width < 3 || height < 3 || data.length < width * height * 4) return null;
  const grey = new Float64Array(width * height);
  for (let i = 0; i < grey.length; i++)
    grey[i] =
      0.2126 * data[i * 4] +
      0.7152 * data[i * 4 + 1] +
      0.0722 * data[i * 4 + 2];
  let sum = 0,
    sq = 0,
    n = 0;
  for (let y = 1; y < height - 1; y++)
    for (let x = 1; x < width - 1; x++) {
      const i = y * width + x,
        v =
          grey[i - 1] +
          grey[i + 1] +
          grey[i - width] +
          grey[i + width] -
          4 * grey[i];
      sum += v;
      sq += v * v;
      n++;
    }
  return Math.max(0, sq / n - (sum / n) ** 2);
}
export function fittedRect(width, height, mediaWidth, mediaHeight) {
  if (!width || !height || !mediaWidth || !mediaHeight)
    return { x: 0, y: 0, width: 0, height: 0 };
  const scale = Math.min(width / mediaWidth, height / mediaHeight),
    w = mediaWidth * scale,
    h = mediaHeight * scale;
  return { x: (width - w) / 2, y: (height - h) / 2, width: w, height: h };
}
