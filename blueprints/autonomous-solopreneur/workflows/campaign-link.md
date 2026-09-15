# Workflow: Marketing Campaign Short Link Provisioning

## Objective
Enable the solo founder to launch trackable marketing campaigns across social media, newsletters, and ad networks in seconds, with standardized UTM tracking, custom slugs, vanity domains, and expiration guardrails.

## Trigger Prompt Examples
- *"Create a short link for our Product Hunt launch pointing to https://myproduct.com with full UTM tags"*
- *"Generate a tracked link for my Twitter bio pointing to https://github.com/myorg/repo with slug 'repo'"*
- *"Create a temporary webinar access link that expires next Friday"*

## Execution Protocol

### Step 1: Analyze Campaign Context & Construct UTM Parameters
Ensure standard UTM taxonomy is applied to the destination URL:
- `utm_source`: Traffic channel (e.g. `twitter`, `producthunt`, `reddit`, `newsletter`)
- `utm_medium`: Delivery mechanism (e.g. `social`, `post`, `email`, `cpc`)
- `utm_campaign`: Campaign name (e.g. `v1_launch`, `summer_promo`)
- `utm_content`: Optional variant identifier (e.g. `header_cta`, `comment_link`)

Compose the final destination URL:
```text
https://myproduct.com/app?utm_source=producthunt&utm_medium=launch&utm_campaign=v1_release
```

### Step 2: Validate Slug & Domain
- If the founder specifies a slug (e.g. `ph-launch`), check if it's available.
- If no slug is specified, generate a clean, readable alphanumeric slug (e.g. `launch-2026`).
- If a custom host is configured (e.g. `go.mybrand.com`), set `host`.

### Step 3: Invoke `create_shortlink`
Execute the tool call:
```json
{
  "destination": "https://myproduct.com/app?utm_source=producthunt&utm_medium=launch&utm_campaign=v1_release",
  "slug": "ph-launch",
  "host": "go.mybrand.com",
  "expiresAt": "2026-10-01T00:00:00Z"
}
```

### Step 4: Output Synthesis

Present the link and tracking metadata:

```markdown
🚀 **Campaign Short Link Created!**

- **Public URL**: `https://go.mybrand.com/ph-launch`
- **Destination**: `https://myproduct.com/app?utm_source=producthunt&utm_medium=launch&utm_campaign=v1_release`
- **Slug**: `ph-launch`
- **Expires**: `2026-10-01 00:00:00 UTC`
- **Analytics Ready**: Real-time click stream, bot filtering, and geo/device tracking enabled on Octarq.
```
