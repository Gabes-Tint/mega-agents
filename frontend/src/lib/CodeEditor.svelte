<script lang="ts">
  import { Annotation, Compartment, EditorState } from "@codemirror/state";
  import { EditorView, placeholder as placeholderText } from "@codemirror/view";
  import { untrack } from "svelte";
  import type { EditorLanguage, VariableCompletion } from "./completions.js";
  import { editorExtensions } from "./editorExtensions.js";

  let {
    value,
    onchange,
    language = "text",
    label,
    placeholder = "",
    minLines = 3,
    maxLines = 20,
    completions = () => [],
  }: {
    value: string;
    onchange: (value: string) => void;
    language?: EditorLanguage;
    label: string;
    placeholder?: string;
    minLines?: number;
    maxLines?: number;
    // Read when the popup opens rather than on every keystroke, so the panel
    // never walks the graph for a field nobody is completing in.
    completions?: () => readonly VariableCompletion[];
  } = $props();

  // Marks the edits this component makes itself, so a field following the
  // selected block does not report its own text back as a change.
  const syncing = Annotation.define<boolean>();
  const naming = new Compartment();
  const syntax = new Compartment();

  let host = $state<HTMLDivElement>();
  let view: EditorView | undefined;

  $effect(() => {
    const parent = host;
    if (!parent) return;
    const editor = new EditorView({
      parent,
      state: EditorState.create({
        doc: untrack(() => value),
        extensions: [
          naming.of(
            EditorView.contentAttributes.of({
              "aria-label": untrack(() => label),
            }),
          ),
          syntax.of(
            editorExtensions(
              untrack(() => language),
              () => completions(),
            ),
          ),
          placeholderText(untrack(() => placeholder)),
          EditorView.updateListener.of((update) => {
            if (!update.docChanged) return;
            if (
              update.transactions.some(
                (transaction) => transaction.annotation(syncing) === true,
              )
            )
              return;
            onchange(update.state.doc.toString());
          }),
        ],
      }),
    });
    view = editor;
    return () => {
      view = undefined;
      editor.destroy();
    };
  });

  // The panel keeps one editor per field and points it at whichever block is
  // selected, so the document follows the property it shows.
  $effect(() => {
    const next = value;
    const editor = view;
    if (!editor || editor.state.doc.toString() === next) return;
    editor.dispatch({
      changes: { from: 0, to: editor.state.doc.length, insert: next },
      annotations: syncing.of(true),
    });
  });

  $effect(() => {
    view?.dispatch({
      effects: naming.reconfigure(
        EditorView.contentAttributes.of({ "aria-label": label }),
      ),
    });
  });

  $effect(() => {
    view?.dispatch({
      effects: syntax.reconfigure(
        editorExtensions(language, () => completions()),
      ),
    });
  });
</script>

<div
  class="editor"
  style:--min-lines={minLines}
  style:--max-lines={maxLines}
  bind:this={host}
></div>

<style>
  .editor {
    border: 1px solid var(--border);
    border-radius: var(--radius);
    background: var(--surface-sunken);
    overflow: hidden;
  }

  .editor:focus-within {
    border-color: var(--accent);
    box-shadow: 0 0 0 3px var(--focus);
  }

  .editor :global(.cm-editor) {
    background: transparent;
    color: var(--text);
    font-family: var(--font-mono);
    font-size: 12px;
  }

  /* CodeMirror draws its own focus ring on the wrapper; the field already
     shows focus, so the inner outline would double it. */
  .editor :global(.cm-editor.cm-focused) {
    outline: none;
  }

  .editor :global(.cm-scroller) {
    font-family: inherit;
    line-height: 1.5;
    max-height: calc(var(--max-lines) * 1.5em);
    overflow-y: auto;
  }

  .editor :global(.cm-content) {
    min-height: calc(var(--min-lines) * 1.5em);
    padding: 0.35rem 0;
    caret-color: var(--text);
  }

  .editor :global(.cm-line) {
    padding: 0 0.5rem;
  }

  .editor :global(.cm-placeholder) {
    color: var(--text-faint);
  }

  .editor :global(.cm-cursor) {
    border-left-color: var(--text);
  }

  .editor :global(.cm-selectionBackground),
  .editor :global(.cm-content ::selection) {
    background: var(--focus);
  }

  .editor :global(.cm-matchingBracket) {
    background: var(--surface-hover);
    outline: 1px solid var(--border-strong);
  }

  /* Syntax classes the shared highlight style hands out. The colours come
     from app.css so the dark theme restates them in one place. */
  .editor :global(.tok-keyword) {
    color: var(--code-keyword);
  }

  .editor :global(.tok-builtin) {
    color: var(--code-builtin);
  }

  .editor :global(.tok-variable) {
    color: var(--code-variable);
  }

  .editor :global(.tok-property) {
    color: var(--code-property);
  }

  .editor :global(.tok-string) {
    color: var(--code-string);
  }

  .editor :global(.tok-number),
  .editor :global(.tok-atom) {
    color: var(--code-number);
  }

  .editor :global(.tok-operator),
  .editor :global(.tok-attribute) {
    color: var(--code-operator);
  }

  .editor :global(.tok-comment) {
    color: var(--code-comment);
    font-style: italic;
  }

  .editor :global(.tok-invalid) {
    color: var(--fail);
  }

  /* A placeholder is filled in before the field's own language sees it, so
     it is marked out of the surrounding syntax. */
  .editor :global(.tok-template) {
    padding: 0 1px;
    border-radius: 3px;
    background: var(--code-template-soft);
    color: var(--code-template);
    font-weight: 600;
  }

  .editor :global(.cm-tooltip-autocomplete) {
    border: 1px solid var(--border-strong);
    border-radius: var(--radius);
    background: var(--surface);
    box-shadow: var(--shadow-lifted);
    color: var(--text);
    font-family: var(--font-ui);
    font-size: 12px;
  }

  .editor :global(.cm-tooltip-autocomplete ul li) {
    padding: 0.2rem 0.45rem;
  }

  .editor :global(.cm-tooltip-autocomplete ul li[aria-selected]) {
    background: var(--accent);
    color: var(--accent-text);
  }

  .editor :global(.cm-completionDetail) {
    margin-left: 0.5rem;
    color: var(--text-faint);
    font-style: normal;
  }

  .editor
    :global(.cm-tooltip-autocomplete ul li[aria-selected])
    :global(.cm-completionDetail) {
    color: var(--accent-text);
  }
</style>
