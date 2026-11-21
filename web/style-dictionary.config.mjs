import StyleDictionary from 'style-dictionary';

// The design is authored in oklch. The stock css group converts every color
// to sRGB hex, which would make the built variables disagree with the design
// source, so that one transform is swapped for the one that keeps the notation.
const cssTransforms = StyleDictionary.hooks.transformGroups.css.map((name) =>
  name === 'color/css' ? 'color/oklch' : name,
);

export default {
  source: ['../design/tokens/*.tokens.json'],
  platforms: {
    css: {
      transforms: cssTransforms,
      buildPath: 'src/styles/',
      files: [
        {
          destination: 'tokens.css',
          format: 'css/variables',
          options: { outputReferences: true },
        },
      ],
    },
  },
};
