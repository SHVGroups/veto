"use client";

import { defaultKeymap, history, historyKeymap, indentWithTab } from "@codemirror/commands";
import { json } from "@codemirror/lang-json";
import { bracketMatching, HighlightStyle, indentOnInput, StreamLanguage, syntaxHighlighting } from "@codemirror/language";
import { Compartment, EditorState, StateEffect, StateField, type Extension } from "@codemirror/state";
import { Decoration, EditorView, highlightActiveLine, keymap, lineNumbers, type DecorationSet } from "@codemirror/view";
import { tags as t } from "@lezer/highlight";
import { forwardRef, useEffect, useImperativeHandle, useRef } from "react";

/** A Cedar tokenizer: enough for highlighting, not a parser. */
const cedar = StreamLanguage.define<{ inComment: boolean }>({
  name: "cedar",
  startState: () => ({ inComment: false }),
  token(stream, state) {
    if (state.inComment) {
      if (stream.skipTo("*/")) {
        stream.next();
        stream.next();
        state.inComment = false;
      } else stream.skipToEnd();
      return "comment";
    }
    if (stream.eatSpace()) return null;
    if (stream.match("//")) {
      stream.skipToEnd();
      return "comment";
    }
    if (stream.match("/*")) {
      state.inComment = true;
      return "comment";
    }
    if (stream.match(/^"(?:[^"\\]|\\.)*"/)) return "string";
    if (stream.match(/^@[A-Za-z_]\w*/)) return "meta";
    if (stream.match(/^-?\d+/)) return "number";
    if (stream.match(/^(permit|forbid)\b/)) return "keyword";
    if (stream.match(/^(when|unless|if|then|else|in|has|like|is)\b/)) return "operatorKeyword";
    if (stream.match(/^(principal|action|resource|context)\b/)) return "variableName.special";
    if (stream.match(/^(true|false)\b/)) return "bool";
    if (stream.match(/^[A-Z]\w*(?=::)/)) return "typeName";
    if (stream.match(/^[A-Za-z_]\w*/)) return "propertyName";
    stream.next();
    return "punctuation";
  },
  languageData: { commentTokens: { line: "//" } },
});

const highlight = HighlightStyle.define([
  { tag: t.comment, color: "var(--ink-3)", fontStyle: "italic" },
  { tag: t.string, color: "var(--proceed)" },
  { tag: t.number, color: "var(--held)" },
  { tag: [t.bool, t.null], color: "var(--safer)" },
  { tag: t.keyword, color: "var(--ink)", fontWeight: "600" },
  { tag: t.operatorKeyword, color: "var(--ink)", fontWeight: "500" },
  { tag: t.meta, color: "var(--safer)" },
  { tag: t.special(t.variableName), color: "var(--held)" },
  { tag: t.typeName, color: "var(--ink)" },
  { tag: [t.propertyName], color: "var(--ink-2)" },
  { tag: t.punctuation, color: "var(--ink-3)" },
]);

const theme = EditorView.theme({
  "&": { backgroundColor: "var(--sunken)", color: "var(--ink)", fontSize: "12.5px", borderRadius: "12px", height: "100%" },
  "&.cm-focused": { outline: "1.5px solid var(--held)" },
  ".cm-scroller": { fontFamily: "var(--font-mono)", lineHeight: "1.7", fontVariantLigatures: "none" },
  ".cm-content": { padding: "12px 0", caretColor: "var(--ink)" },
  ".cm-gutters": { backgroundColor: "transparent", color: "var(--ink-3)", border: "none", paddingLeft: "6px" },
  ".cm-activeLine, .cm-activeLineGutter": { backgroundColor: "color-mix(in srgb, var(--hover) 70%, transparent)" },
  ".cm-selectionBackground, &.cm-focused .cm-selectionBackground": {
    backgroundColor: "color-mix(in srgb, var(--held) 22%, transparent) !important",
  },
  ".cm-cursor": { borderLeftColor: "var(--ink)" },
  ".cm-error-line": { backgroundColor: "var(--stop-wash)" },
});

const setErrors = StateEffect.define<number[]>();
const errorLines = StateField.define<DecorationSet>({
  create: () => Decoration.none,
  update(deco, tr) {
    for (const e of tr.effects) {
      if (e.is(setErrors)) {
        const lines = [...new Set(e.value)].filter((l) => l >= 1 && l <= tr.state.doc.lines).sort((a, b) => a - b);
        return Decoration.set(lines.map((l) => Decoration.line({ class: "cm-error-line" }).range(tr.state.doc.line(l).from)));
      }
    }
    return deco.map(tr.changes);
  },
  provide: (f) => EditorView.decorations.from(f),
});

export interface CodeEditorHandle {
  goToLine: (line: number, column?: number) => void;
}

/** A code editor for Cedar policies and JSON documents. */
export const CodeEditor = forwardRef<
  CodeEditorHandle,
  {
    value: string;
    onChange?: (v: string) => void;
    language: "cedar" | "json";
    readOnly?: boolean;
    errorLines?: number[];
    minHeight?: number;
    label: string;
  }
>(function CodeEditor({ value, onChange, language, readOnly = false, errorLines: errs = [], minHeight = 360, label }, ref) {
  const host = useRef<HTMLDivElement>(null);
  const view = useRef<EditorView | null>(null);
  const editable = useRef(new Compartment());
  const onChangeRef = useRef(onChange);
  useEffect(() => {
    onChangeRef.current = onChange;
  });

  useEffect(() => {
    if (!host.current) return;
    const exts: Extension[] = [
      lineNumbers(),
      history(),
      indentOnInput(),
      bracketMatching(),
      highlightActiveLine(),
      keymap.of([...defaultKeymap, ...historyKeymap, indentWithTab]),
      language === "cedar" ? cedar : json(),
      syntaxHighlighting(highlight),
      theme,
      errorLines,
      EditorView.lineWrapping,
      EditorView.contentAttributes.of({ "aria-label": label }),
      editable.current.of([EditorState.readOnly.of(readOnly), EditorView.editable.of(!readOnly)]),
      EditorView.updateListener.of((u) => {
        if (u.docChanged) onChangeRef.current?.(u.state.doc.toString());
      }),
    ];
    const v = new EditorView({ parent: host.current, state: EditorState.create({ doc: value, extensions: exts }) });
    view.current = v;
    return () => v.destroy();
    // The editor is created once per language; value and readOnly are synced below.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [language]);

  useEffect(() => {
    const v = view.current;
    if (v && v.state.doc.toString() !== value) v.dispatch({ changes: { from: 0, to: v.state.doc.length, insert: value } });
  }, [value]);

  useEffect(() => {
    view.current?.dispatch({
      effects: editable.current.reconfigure([EditorState.readOnly.of(readOnly), EditorView.editable.of(!readOnly)]),
    });
  }, [readOnly]);

  const errKey = errs.join(",");
  useEffect(() => {
    view.current?.dispatch({ effects: setErrors.of(errKey ? errKey.split(",").map(Number) : []) });
  }, [errKey]);

  useImperativeHandle(ref, () => ({
    goToLine(line, column = 1) {
      const v = view.current;
      if (!v || line < 1 || line > v.state.doc.lines) return;
      const l = v.state.doc.line(line);
      const pos = Math.min(l.to, l.from + Math.max(0, column - 1));
      v.dispatch({ selection: { anchor: pos }, scrollIntoView: true });
      v.focus();
    },
  }));

  return <div ref={host} style={{ minHeight }} className="[&_.cm-editor]:min-h-[inherit]" />;
});
