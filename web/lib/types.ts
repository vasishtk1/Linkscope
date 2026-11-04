export type Backlink = {
  domain: string;
  pages: number;
  nofollowPages: number;
  exampleSource: string;
  exampleTarget: string;
};

export type Lookup = {
  domain: string;
  score: number | null;
  linkingDomains: number;
  linkingPages: number;
  nofollowPages: number;
  backlinks: Backlink[];
  source: string;
};
