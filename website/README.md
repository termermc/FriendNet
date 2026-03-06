# Website

The FriendNet website.

# Prerequisites

- Node.js 24.x or later

# Usage

To build the site, run `npm run build`.

To run the site in development mode, run `npm run dev`.

To format all code with Prettier, run `npm run fmt`.

# TypeScript

This template uses TypeScript, but takes advantage of Node's built-in type stripping feature
to run TypeScript code directly without a build step.

This allows hot reloading to work without resorting to a complicated prebuild system.

# IDE Integration

Wunphile uses ES6 template literals for composing HTML.
Different IDEs need different configuration to provide syntax highlighting and intellisense for HTML inside template literals.

## VS Code

Install the [es6-string-html](https://marketplace.visualstudio.com/items?itemName=Tobermory.es6-string-html) extension.

## JetBrains IDEs (WebStorm, IntelliJ IDEA, etc.)

Recent versions of JetBrains IDEs should support syntax highlighting and intellisense for HTML inside template literals out of the box.
