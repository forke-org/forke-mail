/**
 * @fileoverview Forke Platform
 * @copyright (c) 2026 Forke Inc. (https://www.forke.space/)
 *
 * Source-Available License (Non-Commercial / Fair Source).
 * This source code is open for inspection, learning, and personal development.
 * Commercial use, hosting, or resale as a paid service without an explicit
 * commercial license from Forke Inc. is strictly prohibited.
 */

export type FieldKind = 'text' | 'date' | 'status' | 'tag' | 'event' | 'data';

export const baseFields: { value: string; label: string; kind: FieldKind }[] = [
  { value: 'email', label: 'Email', kind: 'text' },
  { value: 'first_name', label: 'First name', kind: 'text' },
  { value: 'last_name', label: 'Last name', kind: 'text' },
  { value: 'status', label: 'Status', kind: 'status' },
  { value: 'created_at', label: 'Added', kind: 'date' },
  { value: 'last_engaged_at', label: 'Last opened or clicked', kind: 'date' },
  { value: 'tag', label: 'Tag', kind: 'tag' },
  { value: 'event', label: 'Event', kind: 'event' },
];

export const ops: Record<FieldKind, [string, string][]> = {
  text: [['equals', 'is'], ['not_equals', 'is not'], ['contains', 'contains'], ['not_contains', 'does not contain'], ['is_set', 'is set'], ['is_not_set', 'is empty']],
  data: [['equals', 'is'], ['not_equals', 'is not'], ['contains', 'contains'], ['not_contains', 'does not contain'], ['greater_than', 'is more than'], ['less_than', 'is less than'], ['is_set', 'is set'], ['is_not_set', 'is not set']],
  date: [['within_days', 'in the last'], ['older_than_days', 'more than … ago'], ['after', 'after'], ['before', 'before'], ['is_set', 'is set'], ['is_not_set', 'never']],
  status: [['equals', 'is'], ['not_equals', 'is not']],
  tag: [['has', 'has'], ['not_has', 'does not have']],
  event: [['performed', 'did'], ['not_performed', 'did not do']],
};

export function kindOf(field: string): FieldKind {
  if (field.startsWith('data.')) return 'data';
  return baseFields.find((f) => f.value === field)?.kind || 'text';
}

export function needsValue(op: string) {
  return !['is_set', 'is_not_set', 'within_days', 'older_than_days'].includes(op);
}

export function needsDays(kind: FieldKind, op: string) {
  return op === 'within_days' || op === 'older_than_days' || kind === 'event';
}
