import { Annotation, Compartment, EditorState } from "@codemirror/state";
import { EditorView, placeholder as placeholderText } from "@codemirror/view";
import type { EditorLanguage, VariableCompletion } from "./completions.js";
import { editorExtensions } from "./editorExtensions.js";

// Everything CodeMirror reaches this module and nothing else imports it
// statically, so the bundler keeps the editor in a chunk of its own and the
// workspace pays for it only once a field asks for one.

// What a field hands the editor when it takes over from the plain text box
// the field showed while this code was still on its way.
export interface EditorMount {
  parent: HTMLElement;
  doc: string;
  // Where the caret sat in the box, so the swap does not move it.
  selection: { anchor: number; head: number };
  label: string;
  placeholder: string;
  language: EditorLanguage;
  completions: () => readonly VariableCompletion[];
  onchange: (value: string) => void;
}

// The handle a field keeps: the editor answers to the same properties the
// component takes, without the component knowing any CodeMirror.
export interface MountedEditor {
  focus(): void;
  setValue(value: string): void;
  setLabel(label: string): void;
  setSyntax(
    language: EditorLanguage,
    completions: () => readonly VariableCompletion[],
  ): void;
  destroy(): void;
}

// Marks the edits the field makes itself, so a field following the selected
// block does not report its own text back as a change.
const syncing = Annotation.define<boolean>();

function labelling(label: string) {
  return EditorView.contentAttributes.of({ "aria-label": label });
}

export function mountEditor(mount: EditorMount): MountedEditor {
  const naming = new Compartment();
  const syntax = new Compartment();
  const anchor = Math.min(mount.selection.anchor, mount.doc.length);
  const head = Math.min(mount.selection.head, mount.doc.length);
  const view = new EditorView({
    parent: mount.parent,
    state: EditorState.create({
      doc: mount.doc,
      selection: { anchor, head },
      extensions: [
        naming.of(labelling(mount.label)),
        syntax.of(editorExtensions(mount.language, mount.completions)),
        placeholderText(mount.placeholder),
        EditorView.updateListener.of((update) => {
          if (!update.docChanged) return;
          if (
            update.transactions.some(
              (transaction) => transaction.annotation(syncing) === true,
            )
          )
            return;
          mount.onchange(update.state.doc.toString());
        }),
      ],
    }),
  });

  return {
    focus: () => view.focus(),
    setValue(value) {
      if (view.state.doc.toString() === value) return;
      view.dispatch({
        changes: { from: 0, to: view.state.doc.length, insert: value },
        annotations: syncing.of(true),
      });
    },
    setLabel: (label) =>
      view.dispatch({ effects: naming.reconfigure(labelling(label)) }),
    setSyntax: (language, completions) =>
      view.dispatch({
        effects: syntax.reconfigure(editorExtensions(language, completions)),
      }),
    destroy: () => view.destroy(),
  };
}
