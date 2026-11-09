try {
  var storedTheme = localStorage.getItem('forkemail.theme');
  document.documentElement.dataset.theme = storedTheme === 'light' || storedTheme === 'dark' ? storedTheme : 'dark';
} catch (e) {
  document.documentElement.dataset.theme = 'dark';
}
