import { Head, Html, Main, NextScript } from 'next/document';

export default function Document() {
  return (
    <Html lang="en">
      <Head>
        <link rel="icon" href="/forke-mark.png" type="image/png" />
        <meta name="theme-color" content="#151414" />
      </Head>
      <body>
        <Main />
        <NextScript />
      </body>
    </Html>
  );
}
