/** @type {import('tailwindcss').Config} */
module.exports = {
  content: ["./src/**/*.html.got"],
  theme: {
    extend: {},
  },
  plugins: [require("@tailwindcss/typography"), require("daisyui")],
  daisyui: {
    themes: ["corporate"],
  },
}
