<script lang="ts">
  import { untrack } from "svelte";
  import type { EditorLanguage, VariableCompletion } from "./completions.js";
  // Type only: the editor's code is fetched when a field first mounts, so
  // nothing here reaches CodeMirror at load time.
  import type { MountedEditor } from "./codeEditorView.js";

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

  let host = $state<HTMLDivElement>();
  let box = $state<HTMLTextAreaElement>();
  let view = $state<MountedEditor>();
  let missing = $state(false);

  // The field is a plain text box until the editor's chunk arrives. The swap
  // reads the box rather than the property: a keystroke the parent has not
  // echoed back yet is still in the box, and the caret and the focus are
  // carried over in the same turn, so nothing typed is lost.
  $effect(() => {
    const parent = host;
    if (!parent) return;
    let mounted: MountedEditor | undefined;
    let dropped = false;
    import("./codeEditorView.js")
      .then(({ mountEditor }) => {
        if (dropped) return;
        const typing = box;
        const doc = typing ? typing.value : untrack(() => value);
        const caret = typing?.selectionStart ?? doc.length;
        const upTo = typing?.selectionEnd ?? caret;
        const inUse = typing !== undefined && document.activeElement === typing;
        // The box is removed on the next render; dropping its name first
        // keeps the field from answering to its label twice meanwhile.
        typing?.removeAttribute("aria-label");
        mounted = mountEditor({
          parent,
          doc,
          selection: { anchor: caret, head: upTo },
          label: untrack(() => label),
          placeholder: untrack(() => placeholder),
          language: untrack(() => language),
          completions: () => completions(),
          onchange: (next) => onchange(next),
        });
        view = mounted;
        if (inUse) mounted.focus();
      })
      .catch(() => {
        if (!dropped) missing = true;
      });
    return () => {
      dropped = true;
      view = undefined;
      mounted?.destroy();
    };
  });

  // The panel keeps one field per property and points it at whichever block
  // is selected, so the text follows the property it shows. Only the property
  // is watched: the swap itself must not write the property back over text
  // the box holds and the parent has not been told about yet. The box is
  // written only when it holds something else, so echoing a keystroke back
  // does not move the caret to the end of what is being typed.
  $effect(() => {
    const next = value;
    const editor = untrack(() => view);
    if (editor) {
      editor.setValue(next);
      return;
    }
    const typing = untrack(() => box);
    if (!typing || typing.value === next) return;
    const caret = Math.min(typing.selectionStart, next.length);
    const upTo = Math.min(typing.selectionEnd, next.length);
    typing.value = next;
    typing.setSelectionRange(caret, upTo);
  });

  $effect(() => view?.setLabel(label));

  $effect(() => view?.setSyntax(language, () => completions()));
</script>

<div
  class="editor"
  style:--min-lines={minLines}
  style:--max-lines={maxLines}
  bind:this={host}
>
  {#if !view}
    <textarea
      class="plain"
      aria-label={label}
      {placeholder}
      spellcheck="false"
      bind:this={box}
      oninput={(event) => onchange(event.currentTarget.value)}
    ></textarea>
  {/if}
</div>
{#if missing}
  <p class="missing" role="status">
    The highlighted editor could not load; this field takes plain text.
  </p>
{/if}

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

  /* What the field is until the editor's chunk arrives: the same box, the
     same text, the same size, without the highlighting. */
  .plain {
    display: block;
    width: 100%;
    box-sizing: border-box;
    padding: 0.35rem 0.5rem;
    border: none;
    background: transparent;
    color: var(--text);
    font-family: var(--font-mono);
    font-size: 12px;
    line-height: 1.5;
    min-height: calc(var(--min-lines) * 1.5em + 0.7rem);
    max-height: calc(var(--max-lines) * 1.5em + 0.7rem);
    resize: none;
  }

  .plain:focus {
    outline: none;
  }

  .plain::placeholder {
    color: var(--text-faint);
  }

  .missing {
    margin: 0.35rem 0 0;
    color: var(--text-faint);
    font-size: 11px;
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
