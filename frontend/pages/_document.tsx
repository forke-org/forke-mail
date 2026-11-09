import { Head, Html, Main, NextScript } from 'next/document';

export default function Document() {
  return (
    <Html lang="en" data-theme="dark">
      <Head>
        <link rel="icon" href="/forke-mark.png" type="image/png" />
        <meta name="theme-color" content="#050505" />
        <script src="/theme.js" />
      </Head>
      <body>
        <Main />
        <NextScript />
      </body>
    </Html>
  );
}
