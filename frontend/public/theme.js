try {
  var storedTheme = localStorage.getItem('forkemail.theme');
  document.documentElement.dataset.theme = storedTheme === 'dark' || storedTheme === 'light'
    ? storedTheme
    : window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
} catch (e) {
  document.documentElement.dataset.theme = 'light';
}
