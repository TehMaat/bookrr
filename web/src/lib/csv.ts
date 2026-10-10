/**
 * Parses CSV/TSV text as exported by Excel, LibreOffice or Google Sheets.
 * The delimiter (tab, ";" or ",") is guessed from the header line; quoted
 * fields may contain delimiters, newlines and doubled quotes.
 */
export function parseCsv(text: string): string[][] {
  text = text.replace(/^﻿/, "")
  const header = text.split(/\r?\n/, 1)[0] ?? ""
  const delim = ["\t", ";", ","].reduce((best, d) => (header.split(d).length > header.split(best).length ? d : best), ",")

  const rows: string[][] = []
  let row: string[] = []
  let field = ""
  let quoted = false
  for (let i = 0; i < text.length; i++) {
    const c = text[i]
    if (quoted) {
      if (c === '"' && text[i + 1] === '"') {
        field += '"'
        i++
      } else if (c === '"') {
        quoted = false
      } else {
        field += c
      }
    } else if (c === '"' && field === "") {
      quoted = true
    } else if (c === delim) {
      row.push(field)
      field = ""
    } else if (c === "\n" || c === "\r") {
      if (c === "\r" && text[i + 1] === "\n") i++
      row.push(field)
      rows.push(row)
      row = []
      field = ""
    } else {
      field += c
    }
  }
  if (field !== "" || row.length > 0) {
    row.push(field)
    rows.push(row)
  }
  return rows.filter((r) => r.some((f) => f.trim() !== ""))
}

/** Renders rows as CSV with ";" (what Excel expects with Italian settings). */
export function toCsv(rows: string[][]): string {
  return rows
    .map((r) => r.map((f) => (/[;"\n\r]/.test(f) ? `"${f.replace(/"/g, '""')}"` : f)).join(";"))
    .join("\r\n")
}
