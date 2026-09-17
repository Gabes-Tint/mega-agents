import {
  autocompletion,
  acceptCompletion,
  closeBrackets,
  closeBracketsKeymap,
  completionKeymap,
  type Completion,
  type CompletionContext,
  type CompletionResult,
} from "@codemirror/autocomplete";
import { defaultKeymap, history, historyKeymap } from "@codemirror/commands";
import { json } from "@codemirror/lang-json";
import {
  HighlightStyle,
  StreamLanguage,
  bracketMatching,
  syntaxHighlighting,
} from "@codemirror/language";
import { shell } from "@codemirror/legacy-modes/mode/shell";
import { Prec, type Extension } from "@codemirror/state";
import {
  Decoration,
  EditorView,
  MatchDecorator,
  ViewPlugin,
  keymap,
  type DecorationSet,
  type ViewUpdate,
} from "@codemirror/view";
import { tags } from "@lezer/highlight";
import type { EditorLanguage, VariableCompletion } from "./completions.js";

// Token classes the stylesheet colours, so both themes stay in app.css and a
// test can see that a field is highlighted at all.
const tokenHighlighting = HighlightStyle.define([
  { tag: tags.keyword, class: "tok-keyword" },
  { tag: tags.controlKeyword, class: "tok-keyword" },
  { tag: tags.standard(tags.variableName), class: "tok-builtin" },
  { tag: tags.variableName, class: "tok-variable" },
  { tag: tags.definition(tags.variableName), class: "tok-variable" },
  { tag: tags.special(tags.variableName), class: "tok-variable" },
  { tag: tags.propertyName, class: "tok-property" },
  { tag: tags.string, class: "tok-string" },
  { tag: tags.special(tags.string), class: "tok-string" },
  { tag: tags.quote, class: "tok-string" },
  { tag: tags.number, class: "tok-number" },
  { tag: tags.bool, class: "tok-atom" },
  { tag: tags.null, class: "tok-atom" },
  { tag: tags.atom, class: "tok-atom" },
  { tag: tags.operator, class: "tok-operator" },
  { tag: tags.attributeName, class: "tok-attribute" },
  { tag: tags.comment, class: "tok-comment" },
  { tag: tags.meta, class: "tok-comment" },
  { tag: tags.invalid, class: "tok-invalid" },
]);

// {{placeholder}} spans, marked on top of whatever language the field uses:
// in a command such as `cd {{workspace.path}}` the placeholder is filled in
// before the shell ever sees it, so it must not read as shell text.
const templateMatcher = new MatchDecorator({
  regexp: /\{\{[^{}]*\}\}/g,
  decoration: Decoration.mark({ class: "tok-template" }),
});

const templateHighlighting = ViewPlugin.fromClass(
  class {
    placeholders: DecorationSet;

    constructor(view: EditorView) {
      this.placeholders = templateMatcher.createDeco(view);
    }

    update(update: ViewUpdate): void {
      this.placeholders = templateMatcher.updateDeco(update, this.placeholders);
    }
  },
  { decorations: (plugin) => plugin.placeholders },
);

// What the user has typed that a variable completes: two open braces, or a
// dollar sign with the optional brace shell expansion uses.
const TEMPLATE_TRIGGER = /\{\{[A-Za-z0-9_.-]*$/;
const ENVIRONMENT_TRIGGER = /\$\{?[A-Za-z0-9_]*$/;

// Rank of the first variable offered. Each one after it ranks lower, so the
// list keeps the order the block derives it in — the workspace fields before
// the results of connected blocks — rather than the alphabetical order that
// equally good matches would otherwise fall into.
const FIRST_RANK = 90;

function option(
  label: string,
  variable: VariableCompletion,
  rank: number,
): Completion {
  return {
    label,
    detail: variable.detail,
    type: "variable",
    boost: FIRST_RANK - rank,
  };
}

// Offers the variables that reach the block: placeholders once {{ is open,
// the workspace environment variables after $ or ${, and all of them when
// the completion is asked for with Ctrl-Space.
export function variableCompletionSource(
  variables: () => readonly VariableCompletion[],
): (context: CompletionContext) => CompletionResult | null {
  return (context: CompletionContext): CompletionResult | null => {
    const offered = variables();
    if (offered.length === 0) return null;
    const placeholders = offered.filter((variable) => variable.kind !== "env");
    const environment = offered.filter((variable) => variable.kind === "env");
    const template = context.matchBefore(TEMPLATE_TRIGGER);
    if (template && placeholders.length > 0) {
      // A closing pair right after the cursor belongs to the placeholder
      // being completed, so accepting one never doubles the braces.
      const after = context.state.sliceDoc(context.pos, context.pos + 2);
      return {
        from: template.from,
        to: after === "}}" ? context.pos + 2 : context.pos,
        options: placeholders.map((variable, rank) =>
          option(variable.label, variable, rank),
        ),
      };
    }
    const dollar = context.matchBefore(ENVIRONMENT_TRIGGER);
    if (dollar && environment.length > 0) {
      const braced =
        context.state.sliceDoc(dollar.from, dollar.from + 2) === "${";
      return {
        from: dollar.from,
        options: environment.map((variable, rank) =>
          option(
            braced ? `\${${variable.label.slice(1)}}` : variable.label,
            variable,
            rank,
          ),
        ),
      };
    }
    if (!context.explicit) return null;
    return {
      from: context.pos,
      options: offered.map((variable, rank) =>
        option(variable.label, variable, rank),
      ),
    };
  };
}

// The syntax a field is edited in. A prompt has none of its own: only its
// placeholders are marked.
function languageOf(language: EditorLanguage): Extension[] {
  if (language === "shell") return [StreamLanguage.define(shell)];
  if (language === "json") return [json(), closeBrackets(), bracketMatching()];
  return [];
}

// Everything a property field's editor needs apart from its own document:
// the language, the shared token classes, history, and the variable popup.
// Tab accepts a completion when the popup is open and otherwise stays
// unbound, so it keeps moving focus out of the field.
export function editorExtensions(
  language: EditorLanguage,
  variables: () => readonly VariableCompletion[],
): Extension[] {
  return [
    ...languageOf(language),
    syntaxHighlighting(tokenHighlighting),
    templateHighlighting,
    history(),
    EditorView.lineWrapping,
    autocompletion({
      override: [variableCompletionSource(variables)],
      icons: false,
    }),
    Prec.highest(keymap.of([{ key: "Tab", run: acceptCompletion }])),
    keymap.of([
      ...closeBracketsKeymap,
      ...completionKeymap,
      ...defaultKeymap,
      ...historyKeymap,
    ]),
  ];
}
