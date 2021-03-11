module.exports = {
  plugins: [
    require('tailwindcss')(%q),
    require('autoprefixer'),
    require('@fullhuman/postcss-purgecss')({
      content: [%q+'/*.html'],
      defaultExtractor: (content) => content.match(/[\w-/:]+(?<!:)/g) || [],
    })
  ]
};
