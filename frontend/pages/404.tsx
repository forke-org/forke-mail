import { Compass } from 'lucide-react';
import { Card, Empty, ButtonLink } from '../components/ui';

export default function NotFound() {
  return (
    <Card>
      <Empty icon={Compass} title="Page not found" hint="The page you are looking for does not exist." action={<ButtonLink href="/">Back to overview</ButtonLink>} />
    </Card>
  );
}
