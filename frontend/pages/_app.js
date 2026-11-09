import Head from 'next/head';
import { useRouter } from 'next/router';
import { useEffect } from 'react';
import Layout from '../components/Layout';
import { ConfirmProvider } from '../components/Modal';
import UpgradeModal from '../components/UpgradeModal';
import { Loading } from '../components/ui';
import { SessionProvider, useSession } from '../lib/session';
import { ToastProvider } from '../lib/toast';

const publicPaths = ['/login'];

function Shell({ Component, pageProps }) {
  const { status } = useSession();
  const router = useRouter();
  const isPublic = publicPaths.includes(router.pathname);

  useEffect(() => {
    if (status === 'signedOut' && !isPublic) router.replace('/login/');
    if (status === 'signedIn' && isPublic) router.replace('/');
  }, [status, isPublic, router]);

  if (status === 'loading') return <div className="auth-screen"><Loading /></div>;
  if (isPublic) return status === 'signedOut' ? <Component {...pageProps} /> : null;
  if (status !== 'signedIn') return null;
  return (
    <Layout>
      <Component {...pageProps} />
      <UpgradeModal />
    </Layout>
  );
}

export default function App(props) {
  return (
    <>
      <Head>
        <title>Forke Mail</title>
        <meta name="viewport" content="width=device-width, initial-scale=1" />
      </Head>
      <ToastProvider>
        <SessionProvider>
          <ConfirmProvider>
            <Shell {...props} />
          </ConfirmProvider>
        </SessionProvider>
      </ToastProvider>
    </>
  );
}
