/** @type {import('tailwindcss').Config} */
module.exports = {
  content: ["./src/**/*.html.got"],
  theme: {
    extend: {},
    container: {
      center: true,
    }
  },
  plugins: [require("@tailwindcss/typography"), require("daisyui")],
  daisyui: {
    themes: ["corporate"],
  },
}
