# Community beta

Tessera is ready to be tried by small teams once the release checks below pass on the deployed version. Start with a small group, collect feedback on the first scheduling week, and expand from there.

## What a team can do

Explore the public preview, create a verified owner account, invite teammates, set up locations and qualifications, build and publish weekly rosters, and view personal schedules. Employees can mark unavailable time and request leave; managers can review it without allowing overlapping assignments.

The manager overview includes a first-roster checklist. Invitees need job qualifications assigned in **Team & locations** before they can be scheduled. An owner working alone can schedule their own staff record.

## Check the deployed release

1. Apply migrations through **7**, deploy both projects, and check `/api/readyz` through the frontend domain.
2. In a private browser, open the public preview, then sign up with an inbox you control. Verify the email, sign in, and create a first draft.
3. Invite a teammate, accept using a different browser session, and assign their qualifications. Publish a roster and confirm the teammate sees only their own shifts.
4. Request leave, approve it, and confirm overlapping assignment is rejected. Cancel it and confirm the assignment succeeds. Check the activity history.
5. Test password recovery, sign out, mobile layouts, and both themes. Confirm database backups and the restore process with the hosting provider before relying on real schedules.

## Set expectations

- Published rosters are frozen. Amendments, shift swaps, recurring availability, calendar feeds, and exports remain future work.
- Starter scheduling rules enforce qualifications, no overlapping shifts, and 11 hours of rest. More than 40 weekly hours warns rather than blocks. These are app defaults, not a universal employment policy; policy settings are not exposed in the UI yet.
- Authentication and invitations send email. Leave decisions currently appear in the app, without notification emails.
- Availability blocks time; it does not track leave balances or calculate entitlements.

When sharing Tessera, describe it as a beta and say which workflows you want people to try. Include the live link, repository, current limitations, and a clear way to report bugs without posting personal data or credentials.
