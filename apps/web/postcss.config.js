// Tailwind v4 ships its PostCSS integration as a separate package, and does its
// own vendor prefixing through Lightning CSS — hence no autoprefixer.
export default {
  plugins: {
    '@tailwindcss/postcss': {},
  },
};
