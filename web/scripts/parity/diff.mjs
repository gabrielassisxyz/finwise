import { readFileSync, writeFileSync } from 'node:fs';
import { PNG } from 'pngjs';

// A channel may differ by this much and still count as the same pixel. It
// absorbs anti-aliasing, not a wrong color.
export const CHANNEL_TOLERANCE = 16;

export function readPng(path) {
  return PNG.sync.read(readFileSync(path));
}

export function writePng(path, png) {
  writeFileSync(path, PNG.sync.write(png));
}

export function crop(png, rect) {
  const out = new PNG({ width: rect.width, height: rect.height });
  PNG.bitblt(png, out, rect.x, rect.y, rect.width, rect.height, 0, 0);
  return out;
}

function pixelDiffers(reference, candidate, x, y) {
  const inReference = x < reference.width && y < reference.height;
  const inCandidate = x < candidate.width && y < candidate.height;
  if (!inReference || !inCandidate) return true;
  const r = (reference.width * y + x) * 4;
  const c = (candidate.width * y + x) * 4;
  for (let channel = 0; channel < 3; channel += 1) {
    if (Math.abs(reference.data[r + channel] - candidate.data[c + channel]) > CHANNEL_TOLERANCE) return true;
  }
  return false;
}

/**
 * Counts the pixels that differ. The images may have different sizes: the
 * area one of them lacks counts as differing, so a page that came out taller
 * or shorter is measured instead of refused.
 */
export function diffImages(reference, candidate) {
  const width = Math.max(reference.width, candidate.width);
  const height = Math.max(reference.height, candidate.height);
  const heatmap = new PNG({ width, height });
  let differing = 0;

  for (let y = 0; y < height; y += 1) {
    for (let x = 0; x < width; x += 1) {
      const at = (width * y + x) * 4;
      const differs = pixelDiffers(reference, candidate, x, y);
      if (differs) differing += 1;
      const inReference = x < reference.width && y < reference.height;
      const base = inReference ? reference.data[(reference.width * y + x) * 4] * 0.35 : 0;
      heatmap.data[at] = differs ? 230 : base;
      heatmap.data[at + 1] = differs ? 40 : base;
      heatmap.data[at + 2] = differs ? 40 : base;
      heatmap.data[at + 3] = 255;
    }
  }

  const total = width * height;
  return { differing, total, percent: (100 * differing) / total, heatmap };
}
