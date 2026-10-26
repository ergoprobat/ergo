export default [
  {
    files: ["web/**/*.js"],
    ignores: ["web/vendor/**"],
    linterOptions: {
      noInlineConfig: true,
    },
    rules: {
      "eqeqeq": [
        "error",
        "always"
      ]
    },
  },
];
