import Link from 'next/link';
import { Card, Empty } from '../components/ui';

export default function NotFound() {
  return (
    <Card>
      <Empty title="Page not found" hint="The page you are looking for does not exist." />
      <div className="card-body"><Link className="btn" href="/">Back to dashboard</Link></div>
    </Card>
  );
}
