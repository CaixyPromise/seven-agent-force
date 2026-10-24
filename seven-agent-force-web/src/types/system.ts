export interface SysOrg {
	id?: API.Int64;
  isDeleted?: number;
  createTime?: string;
  updateTime?: string;
  code?: string;
  name?: string;
	parentId?: API.Int64;
  status?: number;
  sortOrder?: number;
	leaderUserId?: API.Int64;
  children?: SysOrg[];
  parentName?: string;
}
