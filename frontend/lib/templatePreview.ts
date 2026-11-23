/**
 * @fileoverview Forke Platform
 * @copyright (c) 2026 Forke Inc. (https://www.forke.space/)
 *
 * Source-Available License (Non-Commercial / Fair Source).
 * This source code is open for inspection, learning, and personal development.
 * Commercial use, hosting, or resale as a paid service without an explicit
 * commercial license from Forke Inc. is strictly prohibited.
 */

const token = /\{\{\{\s*([a-zA-Z0-9_.]+)\s*\}\}\}|\{\{\s*([a-zA-Z0-9_.]+)\s*\}\}/g;

const chipStyle = 'display:inline-block;padding:0 6px;border-radius:6px;background:rgba(255,122,0,0.16);color:#ff9a3c;font-family:ui-monospace,SFMono-Regular,Menlo,monospace;font-size:0.8em;line-height:1.6;letter-spacing:0;text-transform:none;vertical-align:baseline;white-space:nowrap;';
const blockStyle = 'display:block;margin:8px 0;padding:14px 16px;border:1px dashed rgba(255,122,0,0.5);border-radius:10px;background:rgba(255,122,0,0.06);color:#ff9a3c;font-family:ui-monospace,SFMono-Regular,Menlo,monospace;font-size:12px;line-height:1.5;text-align:left;letter-spacing:0;text-transform:none;';

function imagePlaceholder(name: string) {
  const svg = `<svg xmlns="http://www.w3.org/2000/svg" width="1200" height="630"><rect width="100%" height="100%" fill="#1f1410"/><rect x="24" y="24" width="1152" height="582" rx="24" fill="none" stroke="#ff7a00" stroke-opacity="0.5" stroke-dasharray="14 10" stroke-width="3"/><text x="50%" y="50%" fill="#ff9a3c" font-family="monospace" font-size="40" text-anchor="middle" dominant-baseline="middle">${name}</text></svg>`;
  return `data:image/svg+xml;charset=utf-8,${encodeURIComponent(svg)}`;
}

function today() {
  return new Date().toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' });
}

export const automaticVariables = ['date', 'unsubscribe_url'];

export function previewSubject(subject: string, sample: Record<string, string>): string {
  return subject.replace(token, (_m, raw: string | undefined, plain: string | undefined) => {
    const key = raw || plain || '';
    return sample[key] || (key === 'date' ? today() : `‹${key}›`);
  });
}

export function previewHtml(html: string, sample: Record<string, string>): string {
  const values: Record<string, string> = { date: today(), ...sample };
  const filled = html.replace(token, (match, raw: string | undefined, plain: string | undefined) => {
    const key = raw || plain || '';
    const v = values[key];
    if (v === undefined || v === '') return match;
    return raw ? v : v.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
  });
  if (typeof DOMParser === 'undefined' || !filled.includes('{{')) return filled;
  const marked = filled.replace(/\{\{\{\s*([a-zA-Z0-9_.]+)\s*\}\}\}/g, (_m, name: string) => `<!--fm-block:${name}-->`);
  const doc = new DOMParser().parseFromString(marked, 'text/html');
  const comments: Comment[] = [];
  const commentWalker = doc.createTreeWalker(doc.documentElement, NodeFilter.SHOW_COMMENT);
  for (let n = commentWalker.nextNode(); n; n = commentWalker.nextNode()) if ((n.nodeValue || '').startsWith('fm-block:')) comments.push(n as Comment);
  for (const comment of comments) {
    const box = doc.createElement('div');
    box.setAttribute('style', blockStyle);
    box.textContent = `${(comment.nodeValue || '').slice(9)} · HTML filled in when sent`;
    const parent = comment.parentElement?.tagName || '';
    let node: Element = box;
    if (['TABLE', 'TBODY', 'THEAD', 'TFOOT', 'TR'].includes(parent)) {
      const td = doc.createElement('td');
      td.setAttribute('style', 'padding:0 20px;');
      td.appendChild(box);
      node = td;
      if (parent !== 'TR') {
        const tr = doc.createElement('tr');
        tr.appendChild(td);
        node = tr;
      }
    }
    comment.replaceWith(node);
  }
  doc.querySelectorAll('*').forEach((el) => {
    for (const attr of Array.from(el.attributes)) {
      if (!attr.value.includes('{{')) continue;
      const name = (attr.value.match(/\{\{\{?\s*([a-zA-Z0-9_.]+)/) || [])[1] || 'value';
      if (attr.name === 'src') el.setAttribute('src', imagePlaceholder(name));
      else if (attr.name === 'href') el.setAttribute('href', '#');
      else el.setAttribute(attr.name, attr.value.replace(token, (_m, raw, plain) => raw || plain));
    }
  });
  const walker = doc.createTreeWalker(doc.body, NodeFilter.SHOW_TEXT);
  const nodes: Text[] = [];
  for (let n = walker.nextNode(); n; n = walker.nextNode()) if ((n.nodeValue || '').includes('{{')) nodes.push(n as Text);
  for (const node of nodes) {
    const parts = (node.nodeValue || '').split(token);
    const frag = doc.createDocumentFragment();
    for (let i = 0; i < parts.length; i += 3) {
      if (parts[i]) frag.appendChild(doc.createTextNode(parts[i]));
      const raw = parts[i + 1];
      const plain = parts[i + 2];
      if (raw) {
        const box = doc.createElement('div');
        box.setAttribute('style', blockStyle);
        box.textContent = `${raw} · HTML filled in when sent`;
        frag.appendChild(box);
      } else if (plain) {
        const chip = doc.createElement('span');
        chip.setAttribute('style', chipStyle);
        chip.textContent = plain;
        frag.appendChild(chip);
      }
    }
    node.replaceWith(frag);
  }
  return '<!doctype html>' + doc.documentElement.outerHTML;
}
