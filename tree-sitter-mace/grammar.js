/**
 * @file Tree-sitter grammar for the Mace language
 * @author 
 * @license MIT
 */

/// <reference types="tree-sitter-cli/dsl" />
// @ts-check

export default grammar({
  name: "mace",

  rules: {
    // TODO: add the actual grammar rules
    source_file: $ => "hello"
  }
});
