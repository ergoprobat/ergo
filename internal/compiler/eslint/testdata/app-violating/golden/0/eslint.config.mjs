import tseslint from "typescript-eslint";

export default [
  {
    files: ["services/**/*.ts"],
    languageOptions: {
      parser: tseslint.parser,
      parserOptions: {
        projectService: true,
        tsconfigRootDir: process.cwd(),
      },
    },
    plugins: {
      "@typescript-eslint": tseslint.plugin,
    },
    linterOptions: {
      noInlineConfig: true,
    },
    rules: {
      "@typescript-eslint/no-floating-promises": "error"
    },
  },
];
