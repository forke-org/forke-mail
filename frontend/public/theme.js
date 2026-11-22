/**
 * @fileoverview Forke Platform
 * @copyright (c) 2026 Forke Inc. (https://www.forke.space/)
 *
 * Source-Available License (Non-Commercial / Fair Source).
 * This source code is open for inspection, learning, and personal development.
 * Commercial use, hosting, or resale as a paid service without an explicit
 * commercial license from Forke Inc. is strictly prohibited.
 */

try {
  var storedTheme = localStorage.getItem('forkemail.theme');
  document.documentElement.dataset.theme = storedTheme === 'light' || storedTheme === 'dark' ? storedTheme : 'dark';
} catch (e) {
  document.documentElement.dataset.theme = 'dark';
}
