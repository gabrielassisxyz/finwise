/**
 * The color of a budget line's dot. The hue comes from the name, so a line
 * keeps its color everywhere without anyone assigning one.
 */
export function lineDotColor(lineName: string): string {
  let hue = 0;
  for (const character of lineName) hue = (hue * 31 + character.charCodeAt(0)) % 360;
  return `oklch(0.7 0.09 ${hue})`;
}
